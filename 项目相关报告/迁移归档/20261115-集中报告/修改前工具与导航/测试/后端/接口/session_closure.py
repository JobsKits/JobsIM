#!/usr/bin/env python3
"""Real export reader/writer fence timeout must allow only its scoped logout retry."""
import argparse,fcntl,json,pathlib,secrets,ssl,subprocess,time,urllib.request,urllib.error,uuid
p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);args=p.parse_args();info=json.loads(pathlib.Path(args.environment_file).read_text());base=info['serverURL'];ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
assert '/Assets/' in info['runDir'],'Requires the explicit experimental Assets realm'
def call(method,path,body=None,token=None,expect=200):
 global checks
 headers={'Content-Type':'application/json'}
 if token:headers['Authorization']='Bearer '+token
 request=urllib.request.Request(base+path,data=None if body is None else json.dumps(body).encode(),headers=headers,method=method)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=20) as response:status=response.status;data=response.read()
 except urllib.error.HTTPError as error:status=error.code;data=error.read()
 assert status==expect,(method,path,status,'unexpected body redacted');checks+=1;return json.loads(data) if data else None
suffix=secrets.token_hex(4);lock=None
try:
 boot=call('GET','/api/bootstrap');session=call('POST','/api/auth/register',{'username':'closing_'+suffix,'password':'Scoped session closing '+secrets.token_hex(12),'requestId':str(uuid.uuid4()),'deviceName':'closure-fault-test'});token=session['token'];tokens.append(token)
 conversation=call('POST','/api/conversations',{'kind':'self'},token);call('POST','/api/conversations/'+conversation['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Explicit controlled export fixture','restoreEpoch':boot['restoreEpoch'],'sendAdmission':conversation['sendAdmission']},token)
 package=call('POST','/api/me/exports',{'requestId':str(uuid.uuid4())},token,202);job=package['job']
 for _ in range(100):
  job=call('GET','/api/me/exports/'+job['id'],token=token)
  if job['state']=='ready':break
  assert job['state'] in ('requested','building'),'Controlled fixture export failed';time.sleep(.1)
 else:raise AssertionError('Controlled fixture export did not become ready')
 uuid.UUID(job['artifactId']);path=pathlib.Path(info['stateDir'])/'Exports/Locks'/(job['artifactId']+'.lock');lock=open(path,'a+b');fcntl.flock(lock,fcntl.LOCK_EX)
 call('POST','/api/auth/logout',{},token,503)
 call('GET','/api/me',token=token,expect=401);call('GET','/api/auth/context',token=token,expect=401);call('POST','/api/conversations',{'kind':'self'},token,401)
 assert call('GET','/api/auth/csrf',token=token)['csrfToken'],'Only the original close/CSRF scope may continue'
 fcntl.flock(lock,fcntl.LOCK_UN);lock.close();lock=None
 call('POST','/api/auth/logout',{},token);call('GET','/api/me',token=token,expect=401)
 result={'status':'passed','httpAssertions':checks,'realFileFenceTimeout':True,'sessionRejectsNormalRequestsWhileClosing':True,'onlyOriginalCloseAndCSRFContinue':True,'originalLogoutCompletesAfterFenceDrains':True,'scope':'isolated experimental HTTPS/PG/FS fault test; not full I acceptance'};pathlib.Path('测试/后端/报告/设备关闭受限续跑实测.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n');print(json.dumps(result))
finally:
 if lock:fcntl.flock(lock,fcntl.LOCK_UN);lock.close()
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
