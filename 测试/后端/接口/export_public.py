#!/usr/bin/env python3
"""Exercise explicitly enabled export routes and the real public mutation hooks.
Only the provided isolated realm is used. Does not advertise the experiment as
the complete account export or change the capability registry.
"""
import argparse,base64,hashlib,hmac,json,pathlib,secrets,struct,subprocess,time,urllib.error,urllib.request,ssl,uuid

parser=argparse.ArgumentParser();parser.add_argument('--environment-file',required=True);parser.add_argument('--admin-fixture');parser.add_argument('--report-file');args=parser.parse_args()
info=json.loads(pathlib.Path(args.environment_file).read_text());base=info['serverURL'];assert base=='https://127.0.0.1:20443','Only the explicit isolated Assets realm';ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
def call(method,path,body=None,token=None,expect=200):
 global checks
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),method=method,headers=headers)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=30) as response:status=response.status;data=response.read()
 except urllib.error.HTTPError as error:status=error.code;data=error.read()
 assert status==expect,(method,path,status,'unexpected response redacted');checks+=1
 return json.loads(data) if data else None
def ready(job,token):
 until=time.monotonic()+30
 while time.monotonic()<until:
  status=call('GET','/api/me/exports/'+job['id'],token=token)
  if status['state']=='ready':return status
  assert status['state'] in('requested','building'),('export build terminal state',status['state'])
  time.sleep(.05)
 raise AssertionError('Export did not become ready')
def create(token):
 operation=str(uuid.uuid4());result=call('POST','/api/me/exports',{'requestId':operation},token,202);assert result['keyReturned'] and result['decryptionKey'];again=call('POST','/api/me/exports',{'requestId':operation},token);assert 'decryptionKey' not in again and again['job']['id']==result['job']['id'];ready(result['job'],token);return result['job']
def cleared(job,token):
 state=call('GET','/api/me/exports/'+job['id'],token=token);assert state['state'] not in('ready','building','requested') and state['cleanupState']=='cleared'
 prefix=pathlib.Path(info['stateDir'])/'Exports/Artifacts'/job['id']/job['artifactId']/(job['restoreEpoch']+'-1');assert not prefix.exists(),'Private build prefix survives acknowledged cleanup'
def send(token,conversation,text):
 snapshot=next(x for x in call('GET','/api/conversations',token=token)['items'] if x['id']==conversation)
 return call('POST','/api/conversations/'+conversation+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':text,'restoreEpoch':boot['restoreEpoch'],'sendAdmission':snapshot['sendAdmission']},token)
suffix=secrets.token_hex(4);password='Public export test credential '+secrets.token_hex(10)
def register(prefix):
 result=call('POST','/api/auth/register',{'username':prefix+suffix,'password':password,'requestId':str(uuid.uuid4()),'deviceName':'export-public-lifecycle'});tokens.append(result['token']);return result
def login(username):
 result=call('POST','/api/auth/login',{'username':username,'password':password,'requestId':str(uuid.uuid4()),'deviceName':'export-public-renewed'});tokens.append(result['token']);return result
try:
 boot=call('GET','/api/bootstrap');assert boot['features']['chat_export'] is False,'Experiment must not claim full capability'
 a,b=register('exp_pub_a_'),register('exp_pub_b_');at,bt=a['token'],b['token'];aid,bid=a['user']['id'],b['user']['id']
 request=call('POST','/api/contacts/requests',{'userId':bid,'requestId':str(uuid.uuid4())},at);call('POST','/api/contacts/requests/'+request['id']+'/accept',{},bt)
 direct=call('POST','/api/conversations',{'kind':'direct','memberIds':[bid]},at)['id'];message=send(at,direct,'Original selected export text')
 job1=create(at);call('GET','/api/me/exports/'+job1['id'],token=bt,expect=404);ticket=call('POST','/api/me/exports/'+job1['id']+'/download-ticket',{},at);assert ticket['downloadToken']
 edited=call('PATCH','/api/messages/'+message['id'],{'text':'Edited authoritative export text','expectedVersion':message['version'],'requestId':str(uuid.uuid4())},at);assert int(edited['version'])>int(message['version']);cleared(job1,at);call('POST','/api/me/exports/'+job1['id']+'/download-ticket',{},at,409)
 job2=create(at);call('DELETE','/api/messages/'+message['id'],token=at);cleared(job2,at)
 send(at,direct,'New legitimate text after retraction');job3=create(at);call('POST','/api/auth/logout',{},at);call('GET','/api/me/exports/'+job3['id'],token=at,expect=401);a2=login(a['user']['username']);at=a2['token'];cleared(job3,at)
 group=call('POST','/api/conversations',{'kind':'group','title':'Export membership generation '+suffix,'memberIds':[bid],'requestId':str(uuid.uuid4())},at)['id'];send(at,group,'Current authorized group source');bjob=create(bt)
 members=call('GET','/api/conversations/'+group+'/members',token=bt)['items'];generation=next(x['membershipGeneration'] for x in members if x['id']==bid)
 call('DELETE','/api/conversations/'+group+'/members/'+bid+'?membershipGeneration='+generation,token=bt);cleared(bjob,bt);call('GET','/api/conversations/'+group+'/messages',token=bt,expect=403)
 call('POST','/api/conversations/'+group+'/members',{'userId':bid},at);fresh=call('GET','/api/conversations/'+group+'/members',token=bt)['items'];new_generation=next(x['membershipGeneration'] for x in fresh if x['id']==bid);assert int(new_generation)>int(generation)
 call('DELETE','/api/conversations/'+group+'/members/'+bid+'?membershipGeneration='+generation,token=bt,expect=409);assert any(x['id']==bid for x in call('GET','/api/conversations/'+group+'/members',token=bt)['items'])
 admin_checked=False
 if args.admin_fixture:
  fixture=json.loads(pathlib.Path(args.admin_fixture).read_text());secret_text=pathlib.Path(fixture['secretFile']).read_text().strip();secret=base64.b32decode(secret_text+'='*((-len(secret_text))%8));observed=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',"SELECT last_accepted_step FROM auth.mfa_factors f JOIN auth.users u ON u.id=f.user_id WHERE u.username='admin'"],capture_output=True,text=True,check=True);last=int(observed.stdout.strip())
  def code():
   global last
   while last+1>int(time.time())//30+1:time.sleep(1)
   last=max(last+1,int(time.time())//30);digest=hmac.new(secret,struct.pack('>Q',last),hashlib.sha1).digest();offset=digest[-1]&15;return f'{(struct.unpack(">I",digest[offset:offset+4])[0]&0x7fffffff)%1000000:06d}'
  admin=call('POST','/api/auth/login',{'username':fixture['username'],'password':fixture['password'],'requestId':str(uuid.uuid4()),'deviceName':'export-state-admin','otp':code()});admin_token=admin['token'];tokens.append(admin_token)
  job_before_ban=create(bt);current=next(x for x in call('GET','/api/admin/users?q='+b['user']['username'],token=admin_token)['items'] if x['id']==bid)
  def state(desired,extra=None):
   global current
   payload={'state':desired,'reason':'Isolated export source revocation regression','expectedVersion':current['version']}
   if extra:payload.update(extra)
   operation=str(uuid.uuid4());proof=call('POST','/api/auth/step-up',{'action':'account.state','targetId':bid,'requestId':operation,'otp':code(),'payload':payload},admin_token)
   result=call('PATCH','/api/admin/users/'+bid+'/state',dict(payload,requestId=operation,stepUpId=proof['authorizationId']),admin_token);current=next(x for x in call('GET','/api/admin/users?q='+b['user']['username'],token=admin_token)['items'] if x['id']==bid);return result
  assert state('banned',{'permanent':True})['state']=='banned';call('GET','/api/me',token=bt,expect=401)
  prefix=pathlib.Path(info['stateDir'])/'Exports/Artifacts'/job_before_ban['id']/job_before_ban['artifactId']/(job_before_ban['restoreEpoch']+'-1');assert not prefix.exists()
  assert state('active')['state']=='active';call('GET','/api/me',token=bt,expect=401);b2=login(b['user']['username']);bt=b2['token'];cleared(job_before_ban,bt);admin_checked=True
 report={'status':'passed','httpAssertions':checks,'publicExperimentalRoutes':True,'capabilityRemainsFalse':True,'sourceEditCancelsAndClearsPackage':True,'sourceRetractionCancelsAndClearsPackage':True,'deviceLogoutCancelsPackage':True,'groupLeaveCancelsPackage':True,'oldGenerationCannotKickRejoinedMember':True,'adminBanAndUnbanDoNotResurrectPackage':admin_checked,'scope':'public experimental chat slice; full account export, snapshot consistency and all I remain unaccepted'}
 if args.report_file:pathlib.Path(args.report_file).write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n')
 print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
