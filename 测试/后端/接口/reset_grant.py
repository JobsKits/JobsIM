#!/usr/bin/env python3
"""Real password-reset grant purpose, expiry, one-use and session invalidation."""
import argparse,base64,hashlib,hmac,json,os,pathlib,secrets,ssl,struct,subprocess,time,urllib.request,urllib.error,uuid
p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);args=p.parse_args();info=json.loads(pathlib.Path(args.environment_file).read_text());run=pathlib.Path(info['runDir']);assert run.name=='Safety';base=info['serverURL'];ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
fixture=run/'PrivateAdminFixture.json'
if fixture.exists():admin=json.loads(fixture.read_text())
else:
 admin={'username':'admin','password':'Reset operator '+secrets.token_hex(16),'operation':'bootstrap-'+secrets.token_hex(12),'secretFile':str(run/'PrivateAdminTOTP')};fixture.write_text(json.dumps(admin));os.chmod(fixture,0o600)
environment=dict(os.environ,JOBS_IM_DATABASE_URL=info['businessURL'],JOBS_IM_JOURNAL_DATABASE_URL=info['journalURL'],JOBS_IM_BOOTSTRAP_PASSWORD=admin['password'])
initialized=subprocess.run([info['opsBinary'],'--mode','bootstrap-admin','--username','admin','--state-dir',info['stateDir'],'--totp-secret-file',admin['secretFile'],'--operation-id',admin['operation'],'--internal-base-port',str(info['internalBasePort'])],env=environment,capture_output=True)
assert initialized.returncode==0,'Controlled bootstrap failed; credentials redacted'
text=pathlib.Path(admin['secretFile']).read_text().strip();secret=base64.b32decode(text+'='*((-len(text))%8))
def sql(command):
 result=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',command],capture_output=True,text=True,check=True);return result.stdout.strip()
last=int(sql("SELECT last_accepted_step FROM auth.mfa_factors f JOIN auth.users u ON u.id=f.user_id WHERE u.username='admin'"))
def code():
 global last
 while last+1>int(time.time())//30+1:time.sleep(1)
 last=max(last+1,int(time.time())//30);digest=hmac.new(secret,struct.pack('>Q',last),hashlib.sha1).digest();offset=digest[-1]&15;return f'{(struct.unpack(">I",digest[offset:offset+4])[0]&0x7fffffff)%1000000:06d}'
def call(method,path,body=None,token=None,expect=200):
 global checks
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=20) as response:status=response.status;payload=response.read()
 except urllib.error.HTTPError as error:status=error.code;payload=error.read()
 assert status==expect,(method,path,status,'unexpected body redacted');checks+=1;return json.loads(payload) if payload else None
suffix=secrets.token_hex(4);old='Reset target old phrase '+secrets.token_hex(12);new='Reset target new phrase '+secrets.token_hex(12)
try:
 target=call('POST','/api/auth/register',{'username':'reset_target_'+suffix,'password':old,'requestId':str(uuid.uuid4()),'deviceName':'reset-grant-test'});tokens.append(target['token']);user=target['user'];target_id=user['id']
 session=call('POST','/api/auth/login',{'username':'admin','password':admin['password'],'requestId':str(uuid.uuid4()),'deviceName':'reset-admin-test','otp':code()});admin_token=session['token'];tokens.append(admin_token)
 def grant():
  current=next(x for x in call('GET','/api/admin/users?q='+user['username'],token=admin_token)['items'] if x['id']==target_id);request=str(uuid.uuid4());payload={'reason':'Isolated audited password reset','expectedVersion':current['version']};proof=call('POST','/api/auth/step-up',{'action':'account.reset_grant','targetId':target_id,'requestId':request,'otp':code(),'payload':payload},admin_token);body=dict(payload,requestId=request,stepUpId=proof['authorizationId']);result=call('POST','/api/admin/users/'+target_id+'/reset-grants',body,admin_token)
  assert result['credentialAvailable'] and result['credential'];repeated=call('POST','/api/admin/users/'+target_id+'/reset-grants',body,admin_token);assert repeated['id']==result['id'] and repeated['credentialAvailable'] is False and 'credential' not in repeated
  return result
 issued=grant();call('GET','/api/me',token=target['token'],expect=401)
 call('POST','/api/auth/login',{'username':user['username'],'password':old,'requestId':str(uuid.uuid4()),'deviceName':'old-password'},expect=401)
 call('POST','/api/auth/reset-password',{'username':user['username'],'code':issued['credential'],'password':new,'requestId':str(uuid.uuid4())},expect=400)
 reset={'username':user['username'],'grant':issued['credential'],'password':new,'requestId':str(uuid.uuid4())};call('POST','/api/auth/reset-password',reset);call('POST','/api/auth/reset-password',reset)
 call('POST','/api/auth/reset-password',dict(reset,requestId=str(uuid.uuid4())),expect=403)
 fresh=call('POST','/api/auth/login',{'username':user['username'],'password':new,'requestId':str(uuid.uuid4()),'deviceName':'new-password'});tokens.append(fresh['token']);call('GET','/api/me',token=fresh['token'])
 assert sql("SELECT count(*) FROM auth.mfa_factors WHERE revoked_at IS NULL")=='1','Password reset must not replace an administrator MFA factor'
 # Time predicates use the database clock. Expiring this exact synthetic grant is a controlled fixture operation.
 expired=grant();uuid.UUID(expired['id']);sql("UPDATE auth.reset_grants SET expires_at=now()-interval '1 second' WHERE id='"+expired['id']+"'")
 call('POST','/api/auth/reset-password',dict(reset,grant=expired['credential'],requestId=str(uuid.uuid4())),expect=403);call('GET','/api/me',token=fresh['token'],expect=401)
 report={'status':'passed','httpAssertions':checks,'singleUsePasswordPurpose':True,'shortTermExpiryEnforced':True,'issueInvalidatesOldPasswordAndSessions':True,'originalCredentialNotRedisplayed':True,'originalConsumeResultRetry':True,'mfaFactorNotReset':True,'scope':'isolated real HTTPS/PG/journal password-reset slice; no full I acceptance'};(run/'reset-grant-report.json').write_text(json.dumps(report,indent=2));print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
