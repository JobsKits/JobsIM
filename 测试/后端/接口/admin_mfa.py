#!/usr/bin/env python3
"""Exercise real HTTPS, PostgreSQL and independent-journal admin boundaries."""
import argparse,base64,hashlib,hmac,json,os,pathlib,secrets,ssl,struct,subprocess,time,urllib.request,urllib.error,uuid

p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);args=p.parse_args()
info=json.loads(pathlib.Path(args.environment_file).read_text());run=pathlib.Path(info['runDir'])
assert run.name=='Safety','Requires the explicit isolated Safety realm'
base=info['serverURL'];ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
fixture=run/'PrivateAdminFixture.json'
if fixture.exists():admin=json.loads(fixture.read_text())
else:
 admin={'username':'admin','password':'Private admin fixture '+secrets.token_hex(16),'operation':'bootstrap-test-'+secrets.token_hex(8),'secretFile':str(run/'PrivateAdminTOTP')}
 fixture.write_text(json.dumps(admin));os.chmod(fixture,0o600)
environment=dict(os.environ,JOBS_IM_DATABASE_URL=info['businessURL'],JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL'],JOBS_IM_BOOTSTRAP_PASSWORD=admin['password'])
command=[info['opsBinary'],'--mode','bootstrap-admin','--username',admin['username'],'--state-dir',info['stateDir'],'--totp-secret-file',admin['secretFile'],'--operation-id',admin['operation'],'--internal-base-port',str(info['internalBasePort'])]
for _ in range(2):
 result=subprocess.run(command,env=environment,capture_output=True)
 assert result.returncode==0,'Controlled administrator bootstrap/retry failed; private logs retained'
secret=base64.b32decode(pathlib.Path(admin['secretFile']).read_text().strip()+'='*((-len(pathlib.Path(admin['secretFile']).read_text().strip()))%8))
observed=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',"SELECT last_accepted_step FROM auth.mfa_factors f JOIN auth.users u ON u.id=f.user_id WHERE u.username='admin'"],capture_output=True,text=True,check=True)
last=int(observed.stdout.strip())
# Respect a prior run's real per-account authentication window.
while True:
 window=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',"SELECT COALESCE(sum(count),0) FROM auth.rate_windows WHERE subject='user:admin' AND window_start=date_trunc('minute',now())"],capture_output=True,text=True,check=True)
 if int(window.stdout.strip())==0:break
 time.sleep(1)
def otp(step):
 digest=hmac.new(secret,struct.pack('>Q',step),hashlib.sha1).digest();offset=digest[-1]&15
 return f'{(struct.unpack(">I",digest[offset:offset+4])[0]&0x7fffffff)%1000000:06d}'
def next_otp():
 global last
 while last+1>int(time.time())//30+1:time.sleep(min(2,30-time.time()%30+.1))
 last=max(last+1,int(time.time())//30)
 return otp(last)
def call(method,path,body=None,token=None,expect=200):
 global checks
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=20) as response:status=response.status;data=response.read()
 except urllib.error.HTTPError as error:status=error.code;data=error.read()
 assert status==expect,(method,path,status,'unexpected body redacted');checks+=1
 return json.loads(data) if data else None
def bound(action,target,payload,token):
 request=str(uuid.uuid4());proof=call('POST','/api/auth/step-up',{'action':action,'targetId':target,'requestId':request,'otp':next_otp(),'payload':payload},token)
 return dict(payload,requestId=request,stepUpId=proof['authorizationId'])
suffix=secrets.token_hex(4);password='Safety target credential '+secrets.token_hex(12)
try:
 boot=call('GET','/api/bootstrap');target=call('POST','/api/auth/register',{'username':'mfa_target_'+suffix,'password':password,'requestId':str(uuid.uuid4()),'deviceName':'admin-mfa-test'});tokens.append(target['token'])
 login={'username':admin['username'],'password':admin['password'],'requestId':str(uuid.uuid4()),'deviceName':'admin-mfa-test'}
 assert call('POST','/api/auth/login',login,expect=403)['error']['code']=='MFA_REQUIRED'
 assert call('POST','/api/auth/login',dict(login,requestId=str(uuid.uuid4()),otp='nototp'),expect=403)['error']['code']=='MFA_INVALID'
 code=next_otp();session=call('POST','/api/auth/login',dict(login,otp=code));token=session['token'];tokens.append(token)
 # A lost login response is recoverable with the exact original request, but a new login cannot reuse its TOTP step.
 assert call('POST','/api/auth/login',dict(login,otp=code))['deviceId']==session['deviceId']
 assert call('POST','/api/auth/login',dict(login,requestId=str(uuid.uuid4()),otp=code),expect=403)['error']['code']=='MFA_REPLAY'
 friendship=call('POST','/api/contacts/requests',{'userId':session['user']['id'],'requestId':str(uuid.uuid4())},target['token']);call('POST','/api/contacts/requests/'+friendship['id']+'/accept',{},token)
 group=call('POST','/api/conversations',{'kind':'group','title':'Owner lifecycle '+suffix,'memberIds':[session['user']['id']]},target['token'])
 source=call('POST','/api/conversations/'+group['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Retained ordinary group history','restoreEpoch':boot['restoreEpoch'],'sendAdmission':group['sendAdmission']},target['token'])
 caps=call('GET','/api/capabilities',token=token);payload={'enabled':caps['features']['search'],'expectedVersion':caps['versions']['search']};operation=bound('capability.update','search',payload,token)
 assert call('PUT','/api/capabilities/search',dict(operation,enabled=not payload['enabled']),token,403)['error']['code']=='STEP_UP_MISMATCH'
 changed=call('PUT','/api/capabilities/search',operation,token);assert call('PUT','/api/capabilities/search',operation,token)==changed
 call('PUT','/api/capabilities/groups',operation,token,409)
 target_id=target['user']['id'];current=next(x for x in call('GET','/api/admin/users?q=mfa_target_'+suffix,token=token)['items'] if x['id']==target_id)
 def state(desired,extra=None):
  global current
  payload={'state':desired,'reason':'isolated deterministic safety regression','expectedVersion':current['version']}
  if extra:payload.update(extra)
  operation=bound('account.state',target_id,payload,token)
  changed=call('PATCH','/api/admin/users/'+target_id+'/state',operation,token)
  assert call('PATCH','/api/admin/users/'+target_id+'/state',operation,token)==changed
  record=call('GET','/api/admin/operations/'+operation['requestId'],token=token);assert record['state']=='completed'
  current=next(x for x in call('GET','/api/admin/users?q=mfa_target_'+suffix,token=token)['items'] if x['id']==target_id)
  return changed
 assert state('banned',{'permanent':True})['state']=='banned';call('GET','/api/me',token=target['token'],expect=401)
 assert state('tombstoned')['state']=='tombstoned';assert current['stateBeforeTombstone']=='banned'
 restricted=next(x for x in call('GET','/api/conversations',token=token)['items'] if x['id']==group['id']);assert restricted['state']=='owner_unavailable' and restricted['readOnly']
 assert any(x['id']==source['id'] for x in call('GET','/api/conversations/'+group['id']+'/messages',token=token)['items'])
 owner=next(x for x in call('GET','/api/conversations/'+group['id']+'/members',token=token)['items'] if x['id']==target_id);assert owner['state']=='tombstoned' and owner['role']=='owner' and not owner['username']
 call('POST','/api/conversations/'+group['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Must stay read-only','restoreEpoch':boot['restoreEpoch'],'sendAdmission':group['sendAdmission']},token,409)
 assert state('active')['state']=='banned','Tombstone restore must retain the original punishment'
 assert state('active')['state']=='active';call('GET','/api/me',token=target['token'],expect=401)
 fresh=call('POST','/api/auth/login',{'username':target['user']['username'],'password':password,'requestId':str(uuid.uuid4()),'deviceName':'after-unban'});tokens.append(fresh['token'])
 call('PATCH','/api/conversations/'+group['id'],{'title':'Must not resume ownership automatically'},fresh['token'],403)
 call('DELETE','/api/messages/'+source['id'],token=fresh['token'])
 name='Updated profile '+suffix;call('PATCH','/api/me',{'displayName':name},fresh['token']);assert next(x for x in call('GET','/api/users?q=mfa_target_'+suffix,token=token)['items'] if x['id']==target_id)['displayName']==name
 report={'status':'passed','httpAssertions':checks,'totpRequired':True,'totpReplayRejected':True,'bootstrapExactRetry':True,'boundStepUpPayloadAndTarget':True,'originalOperationResultReplay':True,'tombstoneRetainsBan':True,'ownerUnavailableReadOnlyRetainsHistory':True,'restoredOwnerNeedsIndependentGovernance':True,'authorErasureStillAllowed':True,'oldSessionsStayRevokedAfterUnban':True,'profileUsesDirectoryOwner':True,'scope':'isolated HTTPS/PG/journal integration; no full I acceptance'}
 (run/'admin-mfa-report.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
