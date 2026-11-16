#!/usr/bin/env python3
"""Real HTTPS integration for private canonical avatars and minimal reports.
Creates named test accounts only. An optional private administrator fixture adds
step-up governance checks without printing credentials or authenticator codes.
"""
import argparse,base64,concurrent.futures,hashlib,hmac,json,os,pathlib,secrets,ssl,struct,subprocess,time,urllib.error,urllib.request,uuid,zlib

parser=argparse.ArgumentParser();parser.add_argument('--environment-file',required=True);parser.add_argument('--admin-fixture');args=parser.parse_args()
info=json.loads(pathlib.Path(args.environment_file).read_text());base=info['serverURL'];ctx=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
def call(method,path,body=None,token=None,expect=200,raw=None,headers=None):
 global checks
 h={'Content-Type':'application/json'}
 if token:h['Authorization']='Bearer '+token
 if headers:h.update(headers)
 request=urllib.request.Request(base+path,data=raw if raw is not None else(None if body is None else json.dumps(body).encode()),method=method,headers=h)
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=25) as response:status=response.status;data=response.read()
 except urllib.error.HTTPError as error:status=error.code;data=error.read()
 accepted=expect if isinstance(expect,tuple) else(expect,)
 assert status in accepted,(method,path,status,'unexpected response redacted');checks+=1
 return (json.loads(data) if data and not(headers or{}).get('X-Expect-Binary') else data) if data else None
def binary(path,token,expect=200):
 global checks
 request=urllib.request.Request(base+path,headers={'Authorization':'Bearer '+token})
 try:
  with urllib.request.urlopen(request,context=ctx,timeout=15) as response:status=response.status;data=response.read();content_type=response.headers.get('Content-Type')
 except urllib.error.HTTPError as error:status=error.code;data=b'';content_type=''
 assert status==expect,(path,status,'binary response redacted');checks+=1;return data,content_type
def png(rgb):
 def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
 return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',200,100,8,2,0,0,0))+chunk(b'IDAT',zlib.compress(b''.join(b'\0'+bytes(rgb)*200 for _ in range(100))))+chunk(b'IEND',b'')
def upload(token,image,version,operation=None,expect=200):
 boundary='JobsIMAvatar'+secrets.token_hex(12);operation=operation or str(uuid.uuid4())
 fields=b''.join(b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="'+name.encode()+b'"\r\n\r\n'+value.encode()+b'\r\n' for name,value in [('requestId',operation),('expectedVersion',version)])
 raw=fields+b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="file"; filename="static.png"\r\nContent-Type: image/png\r\n\r\n'+image+b'\r\n--'+boundary.encode()+b'--\r\n'
 return call('POST','/api/me/avatar',token=token,raw=raw,headers={'Content-Type':'multipart/form-data; boundary='+boundary},expect=expect)
def jpeg_size(data):
 assert data[:2]==b'\xff\xd8';p=2
 while p+4<=len(data):
  if data[p]!=255:p+=1;continue
  marker=data[p+1];length=int.from_bytes(data[p+2:p+4],'big')
  if marker in (192,193,194):return int.from_bytes(data[p+7:p+9],'big'),int.from_bytes(data[p+5:p+7],'big')
  p+=2+length
 raise AssertionError('Canonical JPEG dimensions missing')
suffix=secrets.token_hex(4);password='Avatar test credential '+secrets.token_hex(10)
def register(prefix):
 result=call('POST','/api/auth/register',{'username':prefix+suffix,'password':password,'requestId':str(uuid.uuid4()),'deviceName':'avatar-reports-integration'});tokens.append(result['token']);return result
try:
 boot=call('GET','/api/bootstrap');a=register('avatar_a');b=register('avatar_b');c=register('avatar_c');at,bt,ct=a['token'],b['token'],c['token']
 me=call('GET','/api/me',token=at);version=me.get('profileVersion','1');operation=str(uuid.uuid4())
 first=upload(at,png((220,40,30)),version,operation);assert first['status']=='applied' and first['current'];user=first['user'];url=user['avatarURL'];thumbnail=user['avatarThumbnailURL']
 data,cty=binary(url,at);assert cty=='image/jpeg' and jpeg_size(data)==(512,512)
 tiny,_=binary(thumbnail,at);assert jpeg_size(tiny)==(128,128)
 assert upload(at,png((220,40,30)),version,operation)['result']==first['result']
 upload(at,png((30,40,220)),version,operation,409)
 binary(url,bt,403);binary(url,ct,403)
 request=call('POST','/api/contacts/requests',{'userId':b['user']['id'],'requestId':str(uuid.uuid4())},at);call('POST','/api/contacts/requests/'+request['id']+'/accept',{},bt);binary(url,bt)
 c_request=call('POST','/api/contacts/requests',{'userId':c['user']['id'],'requestId':str(uuid.uuid4())},at);call('POST','/api/contacts/requests/'+c_request['id']+'/accept',{},ct)
 group=call('POST','/api/conversations',{'kind':'group','title':'Avatar report ACL '+suffix,'memberIds':[c['user']['id']],'requestId':str(uuid.uuid4())},at);call('DELETE','/api/contacts/'+c['user']['id'],token=at);binary(url,ct)
 call('PUT','/api/blocks/'+b['user']['id'],{},at);binary(url,bt,403)
 # Generic chat media routes must not bypass the profile visibility contract.
 binary('/api/media/'+url.rsplit('/',1)[-1],at,404)
 cancelled=str(uuid.uuid4());upload(at,b'not an image',user['profileVersion'],cancelled,400)
 assert call('GET','/api/me/avatar/operations/'+cancelled,token=at)['status']=='cancelled';assert call('GET','/api/me',token=at)['avatarURL']==url
 second=upload(at,png((20,50,230)),user['profileVersion']);assert second['current'] and second['user']['avatarURL']!=url
 original=call('GET','/api/me/avatar/operations/'+operation,token=at);assert original['status']=='applied' and not original['current'] and original['result']==first['result'];binary(url,at,404)
 call('GET','/api/me/avatar/operations/'+operation,token=bt,expect=404)
 parallel_id=str(uuid.uuid4());parallel_version=second['user']['profileVersion']
 with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:parallel=list(pool.map(lambda _:upload(at,png((30,180,70)),parallel_version,parallel_id),range(4)))
 assert all(x['status']=='applied' and x['result']==parallel[0]['result'] for x in parallel),'Concurrent retries must share the original successful publication'
 # Report only a currently authorized live source. No chat body is copied into the governance record.
 msg=call('POST','/api/conversations/'+group['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'explicit selected source','restoreEpoch':boot['restoreEpoch'],'sendAdmission':group['sendAdmission']},at)
 report_body={'targetType':'message','targetId':msg['id'],'reason':'User-selected report reason','requestId':str(uuid.uuid4())};report=call('POST','/api/reports',report_body,ct);assert call('POST','/api/reports',report_body,ct)['id']==report['id']
 call('POST','/api/reports',dict(report_body,reason='different'),ct,409)
 call('POST','/api/reports',dict(report_body,requestId=str(uuid.uuid4())),bt,403)
 own=call('GET','/api/reports',token=ct);assert any(x['id']==report['id'] for x in own['items']) and own['includesChatBodies'] is False
 assert not any(x['id']==report['id'] for x in call('GET','/api/reports',token=at)['items']);call('GET','/api/admin/reports',token=ct,expect=403)
 call('DELETE','/api/messages/'+msg['id'],token=at);call('POST','/api/reports',dict(report_body,requestId=str(uuid.uuid4())),ct,404)
 admin_checked=False
 if args.admin_fixture:
  admin=json.loads(pathlib.Path(args.admin_fixture).read_text());secret_text=pathlib.Path(admin['secretFile']).read_text().strip();secret=base64.b32decode(secret_text+'='*((-len(secret_text))%8))
  observed=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',"SELECT last_accepted_step FROM auth.mfa_factors f JOIN auth.users u ON u.id=f.user_id WHERE u.username='admin'"],capture_output=True,text=True,check=True);last=int(observed.stdout.strip())
  def next_code():
   global last
   while last+1>int(time.time())//30+1:time.sleep(1)
   last=max(last+1,int(time.time())//30);digest=hmac.new(secret,struct.pack('>Q',last),hashlib.sha1).digest();offset=digest[-1]&15;return f'{(struct.unpack(">I",digest[offset:offset+4])[0]&0x7fffffff)%1000000:06d}'
  session=call('POST','/api/auth/login',{'username':admin['username'],'password':admin['password'],'requestId':str(uuid.uuid4()),'deviceName':'report-admin-regression','otp':next_code()});token=session['token'];tokens.append(token)
  visible=call('GET','/api/admin/reports',token=token);assert any(x['id']==report['id'] for x in visible['items']) and visible['includesChatBodies'] is False
  request=str(uuid.uuid4());payload={'state':'reviewed','reason':'Isolated review audit','expectedVersion':report['version']};proof=call('POST','/api/auth/step-up',{'action':'report.resolve','targetId':report['id'],'requestId':request,'otp':next_code(),'payload':payload},token);mutation=dict(payload,requestId=request,stepUpId=proof['authorizationId'])
  call('PATCH','/api/admin/reports/'+report['id'],dict(mutation,state='dismissed'),token,403)
  changed=call('PATCH','/api/admin/reports/'+report['id'],mutation,token);assert changed['state']=='reviewed' and changed['version']!='1';assert call('PATCH','/api/admin/reports/'+report['id'],mutation,token)==changed
  record=call('GET','/api/admin/report-operations/'+request,token=token);assert record['state']=='completed' and record['result']==changed;admin_checked=True
 report={'status':'passed','httpAssertions':checks,'canonicalJPEGOutputs':True,'avatarACL':True,'avatarOriginalResultSurvivesReplacement':True,'invalidImagePreservesOldAvatar':True,'chatMediaCannotBypassAvatarACL':True,'reportSourceAuthorization':True,'reportsDoNotCopyChatBodies':True,'adminStepUpGovernance':admin_checked,'scope':'avatar and minimal report integration only; evidence uploads, appeals, group avatars and full I remain unaccepted'}
 print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
