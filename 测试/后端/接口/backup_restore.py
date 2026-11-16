#!/usr/bin/env python3
"""Run on an isolated JobsIM realm. Stops only its recorded service PID.
The primary user-facing integration realm is never selected implicitly.
"""
import argparse,hashlib,json,os,pathlib,secrets,signal,ssl,subprocess,time,urllib.parse,urllib.request,urllib.error,uuid,wave,struct,math
p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);args=p.parse_args();envfile=pathlib.Path(args.environment_file);info=json.loads(envfile.read_text());run=pathlib.Path(info['runDir']);assert run.name=='Safety','Requires explicit isolated Safety environment';base=info['serverURL'];ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;sourceEpoch=None
def call(method,path,body=None,token=None,expect=200,raw=None,headers=None):
 global checks
 if method=='POST' and '/conversations/' in path and path.endswith('/messages') and body is not None and sourceEpoch is not None:body=dict(body,restoreEpoch=body.get('restoreEpoch',sourceEpoch))
 h={'Content-Type':'application/json'}
 if token:h['Authorization']='Bearer '+token
 if headers:h.update(headers)
 request=urllib.request.Request(base+path,data=raw if raw is not None else(None if body is None else json.dumps(body).encode()),method=method,headers=h)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=15) as r:status=r.status;data=r.read()
 except urllib.error.HTTPError as e:status=e.code;data=e.read()
 assert status==expect,(method,path,status,'unexpected response; secrets redacted');checks+=1
 return json.loads(data) if data else None
suffix=secrets.token_hex(4);pw='Backup test long phrase 983!'
sourceEpoch=call('GET','/api/bootstrap')['restoreEpoch']
def register(prefix):return call('POST','/api/auth/register',{'username':prefix+suffix,'password':pw,'requestId':str(uuid.uuid4()),'deviceName':'backup-regression'})
a=register('backup_a');b=register('backup_b');at=a['token'];bt=b['token']
req=call('POST','/api/contacts/requests',{'userId':b['user']['id'],'requestId':str(uuid.uuid4())},at);call('POST','/api/contacts/requests/'+req['id']+'/accept',{},bt)
conv=call('POST','/api/conversations',{'kind':'direct','memberIds':[b['user']['id']]},at);msg=call('POST','/api/conversations/'+conv['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Must stay retracted after old snapshot restore','sendAdmission':conv['sendAdmission']},at)
edited=call('POST','/api/conversations/'+conv['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Old private text must never reappear after a newer edit','sendAdmission':conv['sendAdmission']},at)
codes=call('POST','/api/auth/recovery-codes',{'password':pw,'requestId':'codes-'+suffix},at)
# Real AAC/M4A generated from an artificial tone, never the user's microphone.
media=run/'Fixture';media.mkdir(exist_ok=True);wav=media/'tone.wav';m4a=media/'tone.m4a'
with wave.open(str(wav),'wb') as out:
 out.setnchannels(1);out.setsampwidth(2);out.setframerate(24000);out.writeframes(b''.join(struct.pack('<h',int(3000*math.sin(2*math.pi*440*i/24000))) for i in range(7200)))
subprocess.run(['/usr/bin/afconvert','-f','m4af','-d','aac','-b','64000',str(wav),str(m4a)],check=True,capture_output=True)
boundary='JobsIMBoundary'+secrets.token_hex(12);blob=m4a.read_bytes();upload=b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="file"; filename="tone.m4a"\r\nContent-Type: audio/mp4\r\n\r\n'+blob+b'\r\n--'+boundary.encode()+b'--\r\n';asset=call('POST','/api/media',token=at,raw=upload,headers={'Content-Type':'multipart/form-data; boundary='+boundary});assert asset['contentType']=='audio/mp4'
audio=call('POST','/api/conversations/'+conv['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'audio','text':'test tone','attachmentId':asset['id'],'sendAdmission':conv['sendAdmission']},at)
with urllib.request.urlopen(urllib.request.Request(base+'/api/media/'+asset['id'],headers={'Authorization':'Bearer '+bt}),context=ctx) as response:
 assert response.headers['Content-Type']=='audio/mp4';assert hashlib.sha256(response.read()).digest()==hashlib.sha256(blob).digest();checks+=1
operatorEnv=dict(os.environ,JOBS_IM_DATABASE_URL=info['businessURL'],JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL']);backupdir=run/'Backups';backupdir.mkdir(exist_ok=True);backup=backupdir/('snapshot-'+suffix+'.jimbackup');recovery=run/'SeparateRecoveryKey'
def ops(arguments,expect=0):
 result=subprocess.run([info['opsBinary']]+arguments,env=operatorEnv,capture_output=True,text=True);assert result.returncode==expect,('operator failed',result.stderr[-300:]);return result
ops(['--mode','backup','--pg-bin',info['pgBin'],'--state-dir',info['stateDir'],'--tls-dir',str(pathlib.Path(info['caPath']).parent),'--file',str(backup),'--key-file',str(recovery)])
# No backup overwrite; a failed backup cannot replace the previously valid set.
before=hashlib.sha256(backup.read_bytes()).hexdigest();ops(['--mode','backup','--pg-bin',info['pgBin'],'--state-dir',info['stateDir'],'--tls-dir',str(pathlib.Path(info['caPath']).parent),'--file',str(backup),'--key-file',str(recovery)],1);assert hashlib.sha256(backup.read_bytes()).hexdigest()==before
call('DELETE','/api/messages/'+msg['id'],token=at)
updated=call('PATCH','/api/messages/'+edited['id'],{'text':'New private text not present in the old snapshot','expectedVersion':edited['version'],'requestId':'edit-'+suffix},at)
reacted=call('POST','/api/messages/'+edited['id']+'/reactions',{'emoji':'👍'},bt);assert int(reacted['version'])>int(updated['version']) and int(reacted['sequence'])>int(updated['sequence'])
newpw='New backup test long phrase 431!';call('POST','/api/auth/recover',{'username':'backup_a'+suffix,'code':codes['codes'][0],'password':newpw,'requestId':'recover-'+suffix})
late=register('after_snapshot');call('GET','/api/me',token=at,expect=401)
os.kill(info['serverPID'],signal.SIGTERM)
# Wait for all owner pools to close; source is deliberately offline before restore.
time.sleep(.8)
parsed=urllib.parse.urlsplit(info['businessURL']);targetName='jobsim_restored_'+suffix;subprocess.run([str(pathlib.Path(info['pgBin'])/'createdb'),'-h',parsed.hostname,'-p',str(parsed.port),'-U','jobsim_test',targetName],check=True);target=urllib.parse.urlunsplit(parsed._replace(path='/'+targetName));operatorEnv['JOBS_IM_RESTORE_DATABASE_URL']=target;state=run/('RestoredState-'+suffix)
restoreArgs=['--mode','restore','--offline','--operation-id','restore-'+suffix,'--pg-bin',info['pgBin'],'--state-dir',info['stateDir'],'--target-state-dir',str(state),'--file',str(backup),'--key-file',str(recovery)]
# Simulate a lost pg_restore completion response after its real transaction has
# committed. The runner must continue the same operation without a second epoch.
wrapped=run/'WrappedPG';wrapped.mkdir(exist_ok=True);wrapper=wrapped/'pg_restore';once=wrapped/('failed-after-commit-'+suffix)
wrapper.write_text('#!/usr/bin/env python3\nimport pathlib,subprocess,sys\nresult=subprocess.run(['+repr(str(pathlib.Path(info['pgBin'])/'pg_restore'))+']+sys.argv[1:])\nmarker=pathlib.Path('+repr(str(once))+')\nif result.returncode==0 and not marker.exists():\n marker.write_text("transaction committed; response intentionally lost")\n sys.exit(75)\nsys.exit(result.returncode)\n');os.chmod(wrapper,0o700)
firstRestore=restoreArgs.copy();firstRestore[firstRestore.index(info['pgBin'])]=str(wrapped);ops(firstRestore,1);assert(state/'restore.pending').exists()
def scalar(url,query):return subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),url,'-Atc',query],check=True,capture_output=True,text=True).stdout.strip()
allocated=scalar(info['journalURL'],"SELECT restore_epoch FROM journal.restore_operations WHERE operation_id='restore-"+suffix+"'")
ops(restoreArgs);ops(restoreArgs)
receipt=json.loads((state/'restore-receipt.json').read_text());assert int(receipt['restoreEpoch'])>1;assert receipt['restoreEpoch']==allocated;assert not(state/'restore.pending').exists();assert receipt['resumable'] and receipt['journalCoverage']['snapshotAnchorVerified'];assert int(receipt['journalCoverage']['through'])>=int(receipt['journalCoverage']['snapshotThrough'])
# Tampering is rejected before changing an empty target database.
corrupt=backupdir/('tampered-'+suffix+'.jimbackup');data=bytearray(backup.read_bytes());data[len(data)//2]^=1;corrupt.write_bytes(data);emptyName='jobsim_reject_'+suffix;subprocess.run([str(pathlib.Path(info['pgBin'])/'createdb'),'-h',parsed.hostname,'-p',str(parsed.port),'-U','jobsim_test',emptyName],check=True);operatorEnv['JOBS_IM_RESTORE_DATABASE_URL']=urllib.parse.urlunsplit(parsed._replace(path='/'+emptyName));badArgs=restoreArgs.copy();badArgs[badArgs.index(str(backup))]=str(corrupt);badArgs[badArgs.index(str(state))]=str(run/('RejectedState-'+suffix));ops(badArgs,1)
bindingArgs=restoreArgs.copy();bindingArgs[bindingArgs.index(str(state))]=str(run/('DifferentBoundState-'+suffix));ops(bindingArgs,1);assert scalar(operatorEnv['JOBS_IM_RESTORE_DATABASE_URL'],"SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN('pg_catalog','information_schema')")=='0'
# Start the isolated restored realm with exactly the recovered instance identity.
log=open(run/'restored-server.log','ab',buffering=0);serverEnv=dict(os.environ,JOBS_IM_DATABASE_URL=target,JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL'],JOBS_IM_PUBLIC_URL=base);server=subprocess.Popen([info['binary'],'--service=all','--addr',urllib.parse.urlsplit(base).netloc,'--internal-base-port',str(info['internalBasePort']),'--state-dir',str(state),'--tls-cert',str(state/'TLS/server.pem'),'--tls-key',str(state/'TLS/server.key')],env=serverEnv,stdout=log,stderr=log,start_new_session=True);info.update({'serverPID':server.pid,'businessURL':target,'stateDir':str(state)});envfile.write_text(json.dumps(info));os.chmod(envfile,0o600)
for _ in range(60):
 try:boot=call('GET','/api/bootstrap');break
 except Exception:time.sleep(.1)
else:raise AssertionError((run/'restored-server.log').read_text()[-1000:])
assert boot['restoreEpoch']==receipt['restoreEpoch'];call('GET','/api/me',token=bt,expect=401)
call('POST','/api/auth/login',{'username':'backup_a'+suffix,'password':pw,'requestId':str(uuid.uuid4()),'deviceName':'restore-check'},expect=401)
call('POST','/api/auth/register',{'username':'after_snapshot'+suffix,'password':pw,'requestId':str(uuid.uuid4()),'deviceName':'restore-check'},expect=409)
fresh=call('POST','/api/auth/login',{'username':'backup_b'+suffix,'password':pw,'requestId':str(uuid.uuid4()),'deviceName':'restore-check'});token=fresh['token'];history=call('GET','/api/conversations/'+conv['id']+'/messages',token=token);restored=next(x for x in history['items'] if x['id']==msg['id']);assert restored['deleted'] and restored['text']==''
unavailable=next(x for x in history['items'] if x['id']==edited['id']);assert unavailable['contentUnavailable'] and unavailable['text']=='' and not unavailable['deleted'];assert int(unavailable['version'])>=int(updated['version'])
call('GET','/api/conversations/'+conv['id']+'/messages?after=1:999',token=token,expect=409)
call('POST','/api/conversations/'+conv['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'old queued','restoreEpoch':'1','sendAdmission':conv['sendAdmission']},token,expect=409)
freshConv=next(x for x in call('GET','/api/conversations',token=token)['items'] if x['id']==conv['id'])
call('POST','/api/conversations/'+conv['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'current epoch','restoreEpoch':boot['restoreEpoch'],'sendAdmission':freshConv['sendAdmission']},token)
report={'status':'passed','httpAssertions':checks,'encryptedBackup':True,'tamperRejected':True,'lastValidSetPreserved':True,'realM4AAudioUploadBindDownload':True,'oldSessionRejected':True,'oldPasswordNotTrusted':True,'withdrawalReplayed':True,'oldEditedTextNotResurrected':True,'reactionVersionAndSequenceAdvanceTogether':True,'reservedUsernamePreserved':True,'oldCursorAndOutboxFrozen':True,'lostDatabaseImportResponseResumed':True,'sameRestoreOperationKeepsEpoch':True,'wrongTargetBindingRejected':True,'journalSnapshotAnchorAndTailVerified':True,'scope':'isolated safety realm; no full I/strong protection acceptance'}
(run/'backup-restore-report.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
