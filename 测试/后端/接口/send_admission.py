#!/usr/bin/env python3
"""Original send-admission snapshots, real HTTPS/PG and a blocked-commit race."""
import argparse,concurrent.futures,hashlib,json,pathlib,secrets,ssl,subprocess,time,urllib.error,urllib.request,uuid

p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);args=p.parse_args()
env=json.loads(pathlib.Path(args.environment_file).read_text());base=env['serverURL'];trust=ssl.create_default_context(cafile=env['caPath']);checks=0;tokens=[]
def call(method,path,body=None,token=None,expect=200):
 global checks
 request=urllib.request.Request(base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else{})})
 try:
  with urllib.request.urlopen(request,context=trust,timeout=25) as response:status=response.status;raw=response.read()
 except urllib.error.HTTPError as error:status=error.code;raw=error.read()
 assert status==expect,(method,path,status,'response redacted');checks+=1;return json.loads(raw) if raw else None
def sql(command):
 result=subprocess.run([str(pathlib.Path(env['pgBin'])/'psql'),env['businessURL'],'-XAtc',command],capture_output=True,text=True)
 assert result.returncode==0,'private fixture query failed; details redacted';return result.stdout.strip()
def register(label):
 result=call('POST','/api/auth/register',{'username':label+secrets.token_hex(5),'password':'Admission integration '+secrets.token_hex(14),'requestId':str(uuid.uuid4()),'deviceName':'send-admission-test'});tokens.append(result['token']);return result
def friend(a,b):
 request=call('POST','/api/contacts/requests',{'userId':b['user']['id'],'requestId':str(uuid.uuid4())},a['token']);call('POST','/api/contacts/requests/'+request['id']+'/accept',{},b['token'])
def snapshot(session,conversation):
 return next(c for c in call('GET','/api/conversations',token=session['token'])['items'] if c['id']==conversation)
def payload(token,text='original payload'):
 return {'clientMsgId':str(uuid.uuid4()),'type':'text','text':text,'restoreEpoch':epoch,**({'sendAdmission':token} if token is not None else{})}
def send(session,conversation,body,expect=200):return call('POST','/api/conversations/'+conversation+'/messages',body,session['token'],expect)
try:
 boot=call('GET','/api/bootstrap');epoch=boot['restoreEpoch'];assert boot['features']['send_admission'] is True
 a,b=register('adm_a_'),register('adm_b_');at,bt=a['token'],b['token'];aid,bid=a['user']['id'],b['user']['id'];friend(a,b)
 assert call('GET','/api/capabilities',token=at)['features']['send_admission'] is True
 direct=call('POST','/api/conversations',{'kind':'direct','memberIds':[bid],'requestId':str(uuid.uuid4())},at);d=direct['id'];old=direct['sendAdmission'];assert old.startswith('sa1.') and len(old)==47 and direct['membershipGeneration']=='1'
 missing=payload(None);assert send(a,d,missing,409)['error']['code']=='SEND_ADMISSION_CHANGED';assert sql("SELECT count(*) FROM message.messages WHERE sender_id='"+aid+"' AND client_msg_id='"+missing['clientMsgId']+"'")=='0'
 original=payload(old);first=send(a,d,original);assert send(a,d,original)['id']==first['id'];assert send(a,d,dict(original,sendAdmission='changed'),409)['error']['code']=='IDEMPOTENCY_CONFLICT'
 wrong=payload(snapshot(b,d)['sendAdmission']);assert send(a,d,wrong,409)['error']['code']=='SEND_ADMISSION_CHANGED'
 # A real pre-upgrade record, with exactly the former Go struct's digest.
 legacy=payload(None,'legacy original without snapshot');legacy_id=str(uuid.uuid4());ordered={key:legacy.get(key,'') for key in ['clientMsgId','type','text','replyToId','attachmentId','forwardOfId','restoreEpoch']};digest=hashlib.sha256(json.dumps({'conversationId':d,'payload':ordered},separators=(',',':')).encode()).hexdigest()
 sql("INSERT INTO message.messages(id,conversation_id,sender_id,client_msg_id,payload_digest,type,text,sequence) VALUES('"+legacy_id+"','"+d+"','"+aid+"','"+legacy['clientMsgId']+"',decode('"+digest+"','hex'),'text','legacy original without snapshot',1)")
 assert send(a,d,legacy)['id']==legacy_id
 call('DELETE','/api/contacts/'+bid,token=at);assert snapshot(a,d)['sendAdmission']=='';assert send(a,d,original)['id']==first['id']
 friend(a,b);fresh=snapshot(a,d)['sendAdmission'];assert fresh!=old
 stale=payload(old);assert send(a,d,stale,409)['error']['code']=='SEND_ADMISSION_CHANGED';send(a,d,payload(fresh))
 # Repeated acceptance/deletion must not fabricate a fresh authorization.
 call('DELETE','/api/contacts/'+bid,token=at);call('DELETE','/api/contacts/'+bid,token=at);friend(a,b);before_block=snapshot(a,d)['sendAdmission']
 before_b=snapshot(b,d)['sendAdmission'];call('PUT','/api/blocks/'+bid,{},at)
 assert snapshot(a,d)['sendAdmission']=='' and snapshot(b,d)['sendAdmission']==''
 for actor,peer in [(at,bid),(bt,aid)]:assert any(item['id']==peer for item in call('GET','/api/contacts',token=actor)['items'])
 assert send(a,d,payload(before_block),409)['error']['code']=='SEND_ADMISSION_CHANGED';assert send(b,d,payload(before_b),409)['error']['code']=='SEND_ADMISSION_CHANGED'
 blocked=call('POST','/api/conversations',{'kind':'direct','memberIds':[bid]},at);assert blocked['id']==d and blocked['sendAdmission']==''
 call('DELETE','/api/blocks/'+bid,token=at);restored=snapshot(a,d)['sendAdmission'];assert restored and restored!=before_block
 assert send(a,d,payload(before_block),409)['error']['code']=='SEND_ADMISSION_CHANGED';send(a,d,payload(restored))
 group=call('POST','/api/conversations',{'kind':'group','title':'Admission scope','memberIds':[bid]},at);g=group['id'];b_old=snapshot(b,g);b_original=payload(b_old['sendAdmission']);send(b,g,b_original)
 members=call('GET','/api/conversations/'+g+'/members',token=at)['items'];generation=next(x['membershipGeneration'] for x in members if x['id']==bid)
 call('DELETE','/api/conversations/'+g+'/members/'+bid+'?membershipGeneration='+generation,token=at);call('POST','/api/conversations/'+g+'/members',{'userId':bid},at);b_new=snapshot(b,g);assert b_new['membershipGeneration']!=generation and b_new['sendAdmission']!=b_old['sendAdmission']
 assert send(b,g,payload(b_old['sendAdmission']),409)['error']['code']=='SEND_ADMISSION_CHANGED';assert send(b,g,b_original,403)['error']['code']=='HISTORY_NOT_ALLOWED';send(b,g,payload(b_new['sendAdmission']))
 assert call('DELETE','/api/conversations/'+g+'/members/'+bid+'?membershipGeneration='+generation,token=at,expect=409)['error']['code'] in ('MEMBERSHIP_CHANGED','MEMBERSHIP_GENERATION_CHANGED')
 # Hold the DB before message insertion. Revocation must wait on the OS fence.
 blocker=subprocess.Popen([str(pathlib.Path(env['pgBin'])/'psql'),env['businessURL'],'-XAt'],stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL,text=True,bufsize=1)
 try:
  blocker.stdin.write("BEGIN; LOCK TABLE message.messages IN ACCESS EXCLUSIVE MODE; SELECT 'locked';\n");blocker.stdin.flush()
  while blocker.stdout.readline().strip()!='locked':pass
  current=snapshot(a,d)['sendAdmission'];raced=payload(current,'serialized before revocation')
  with concurrent.futures.ThreadPoolExecutor(max_workers=2) as pool:
   sending=pool.submit(send,a,d,raced)
   for _ in range(100):
    if int(sql("SELECT count(*) FROM pg_stat_activity WHERE application_name='jobsim-message' AND wait_event_type='Lock'"))>0:break
    time.sleep(.02)
   else:raise AssertionError('send did not reach controlled PG lock')
   removing=pool.submit(call,'DELETE','/api/contacts/'+bid,None,at);time.sleep(.15);assert not removing.done(),'revocation crossed an active send commit fence'
   blocker.stdin.write('COMMIT;\n\\q\n');blocker.stdin.flush();assert sending.result()['clientMsgId']==raced['clientMsgId'];removing.result()
  friend(a,b);assert send(a,d,payload(current),409)['error']['code']=='SEND_ADMISSION_CHANGED'
 finally:
  if blocker.poll() is None:blocker.stdin.close();blocker.wait(timeout=5)
 report={'status':'passed','httpAssertions':checks,'strictHTTPS':True,'legacyCommittedDigestReplay':True,'missingSnapshotCannotCreate':True,'peerTokenCannotBorrow':True,'directDeleteAndBlockRotateGrant':True,'groupRejoinKeepsOldQueueIneligible':True,'originalReadACLBeforeReplay':True,'revocationWaitsForActualCommit':True,'scope':'single-host common-state-dir OS fence; independent-domain PG transactions; throughput and cross-host distribution not accepted'}
 destination=pathlib.Path(__file__).resolve().parents[3]/'项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '发送准入代际实测.json';destination.parent.mkdir(parents=True,exist_ok=True);destination.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
