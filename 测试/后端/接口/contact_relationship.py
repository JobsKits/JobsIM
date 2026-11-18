#!/usr/bin/env python3
"""Real HTTPS/PG checks for authenticated, owner-only relationship metadata."""
import argparse,json,pathlib,secrets,ssl,struct,subprocess,urllib.error,urllib.request,uuid,zlib

parser=argparse.ArgumentParser();parser.add_argument('--environment-file',required=True);args=parser.parse_args()
info=json.loads(pathlib.Path(args.environment_file).read_text());base=info['serverURL'];context=ssl.create_default_context(cafile=info['caPath']);checks=0;tokens=[]
expected_keys={'peerId','blockedByMe','friend','inboundGate','generation','inboundReopened'}

def call(method,path,body=None,token=None,expect=200,binary=False,headers=None):
 global checks
 values={'Content-Type':'application/json'}
 if token:values['Authorization']='Bearer '+token
 if headers:values.update(headers)
 request=urllib.request.Request(base+path,data=body if isinstance(body,bytes) else(None if body is None else json.dumps(body).encode()),headers=values,method=method)
 try:
  with urllib.request.urlopen(request,context=context,timeout=20) as response:status=response.status;data=response.read();cache=response.headers.get('Cache-Control')
 except urllib.error.HTTPError as error:status=error.code;data=error.read();cache=error.headers.get('Cache-Control')
 assert status==expect,(method,path,status,'response redacted');checks+=1
 if method=='GET' and path.split('?')[0].endswith('/relationship') and status==200:assert cache=='no-store'
 return data if binary else(json.loads(data) if data else None)

def relationship(session,peer,query=''):
 value=call('GET','/api/contacts/'+peer+'/relationship'+query,token=session['token'])
 assert set(value)==expected_keys and value['peerId']==peer
 assert isinstance(value['generation'],str) and value['generation'].isdecimal()
 return value

def sql(command):
 result=subprocess.run([str(pathlib.Path(info['pgBin'])/'psql'),info['businessURL'],'-Atc',command],capture_output=True,text=True)
 assert result.returncode==0,'test-owned fixture query failed; details redacted'
 return result.stdout.strip()

def fixture_png():
 def chunk(kind,data):return struct.pack('>I',len(data))+kind+data+struct.pack('>I',zlib.crc32(kind+data)&0xffffffff)
 return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('>IIBBBBB',8,8,8,2,0,0,0))+chunk(b'IDAT',zlib.compress((b'\0'+bytes((25,70,110))*8)*8))+chunk(b'IEND',b'')

def avatar(session):
 version=call('GET','/api/me',token=session['token'])['profileVersion'];boundary='Relationship'+secrets.token_hex(10)
 fields=b''.join(b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="'+name.encode()+b'"\r\n\r\n'+value.encode()+b'\r\n' for name,value in [('requestId',str(uuid.uuid4())),('expectedVersion',version)])
 data=fields+b'--'+boundary.encode()+b'\r\nContent-Disposition: form-data; name="file"; filename="controlled.png"\r\nContent-Type: image/png\r\n\r\n'+fixture_png()+b'\r\n--'+boundary.encode()+b'--\r\n'
 return call('POST','/api/me/avatar',data,session['token'],headers={'Content-Type':'multipart/form-data; boundary='+boundary})['user']['avatarURL']

suffix=secrets.token_hex(5)
def register(label):
 value=call('POST','/api/auth/register',{'username':label+suffix,'password':'Relationship test '+secrets.token_hex(16),'requestId':str(uuid.uuid4()),'deviceName':'relationship-metadata-test'});tokens.append(value['token']);uuid.UUID(value['user']['id']);return value

try:
 a,b=register('rel_a_'),register('rel_b_');aid,bid=a['user']['id'],b['user']['id'];path='/api/contacts/'+bid+'/relationship'
 call('GET',path,expect=401);call('GET','/api/contacts/not-a-uuid/relationship',token=a['token'],expect=400);call('GET','/api/contacts/'+aid+'/relationship',token=a['token'],expect=400)
 default=relationship(a,bid);assert default=={'peerId':bid,'blockedByMe':False,'friend':False,'inboundGate':'open','generation':'0','inboundReopened':False}
 before=sql("SELECT count(*) FROM directory.contacts WHERE owner_id='"+aid+"'")
 assert relationship(a,bid)==default;unknown=str(uuid.uuid4());assert relationship(a,unknown)==dict(default,peerId=unknown)
 assert sql("SELECT count(*) FROM directory.contacts WHERE owner_id='"+aid+"'")==before,'GET must not create default gate rows'
 reopen_a={'requestId':str(uuid.uuid4())};opened=call('POST','/api/contacts/'+bid+'/reopen',reopen_a,a['token']);current_a=relationship(a,bid);assert current_a['generation']==opened['generation'] and current_a['inboundReopened']
 assert call('POST','/api/contacts/'+bid+'/reopen',reopen_a,a['token'])['generation']==opened['generation']
 assert relationship(b,aid)['generation']=='0','Alice cannot reopen Bob\'s gate'
 call('POST','/api/contacts/'+aid+'/reopen',{'requestId':str(uuid.uuid4())},b['token']);assert relationship(a,bid)==current_a
 request=call('POST','/api/contacts/requests',{'userId':bid,'requestId':str(uuid.uuid4())},a['token']);before_reject=relationship(a,bid)
 call('POST','/api/contacts/requests/'+request['id']+'/reject',{},b['token']);assert relationship(a,bid)==before_reject,'peer private gate is not projected into caller metadata'
 b_closed=relationship(b,aid);assert b_closed['inboundGate']=='open' and not b_closed['inboundReopened']
 # Supplying somebody else's owner ID never changes whose data is queried.
 assert relationship(a,bid,'?ownerId='+bid+'&userId='+bid+'&accountId='+bid)==current_a
 # Rejection retires the old request; a fresh explicit round needs no reopen.
 renewed=call('POST','/api/contacts/requests',{'userId':bid,'requestId':str(uuid.uuid4())},a['token']);call('POST','/api/contacts/requests/'+renewed['id']+'/accept',{},b['token']);assert relationship(a,bid)['friend'] and relationship(b,aid)['friend']
 image=avatar(b);assert call('GET',image,token=a['token'],binary=True).startswith(b'\xff\xd8')
 call('PUT','/api/blocks/'+bid,{},a['token']);assert relationship(a,bid)['blockedByMe'] and relationship(a,bid)['friend'];assert not relationship(b,aid)['blockedByMe'] and relationship(b,aid)['friend'];call('GET',image,token=a['token'],expect=403)
 call('PUT','/api/blocks/'+aid,{},b['token']);call('DELETE','/api/blocks/'+bid,token=a['token']);assert not relationship(a,bid)['blockedByMe'] and relationship(b,aid)['blockedByMe'];call('GET',image,token=a['token'],expect=403)
 call('DELETE','/api/blocks/'+aid,token=b['token']);closed=relationship(a,bid);assert not closed['blockedByMe'] and closed['friend'] and closed['inboundGate']=='open' and not closed['inboundReopened'];assert call('GET',image,token=a['token'],binary=True).startswith(b'\xff\xd8')
 assert call('POST','/api/contacts/'+bid+'/reopen',reopen_a,a['token'])['current'] is False;assert relationship(a,bid)==closed,'old successful reopen is not current permission'
 call('DELETE','/api/contacts/'+bid,token=a['token']);assert not relationship(a,bid)['friend'] and not relationship(b,aid)['friend'];call('GET',image,token=a['token'],expect=403)
 call('POST','/api/contacts/requests/'+renewed['id']+'/accept',{},b['token'],expect=409)
 fresh=call('POST','/api/contacts/requests',{'userId':bid,'requestId':str(uuid.uuid4())},a['token']);call('POST','/api/contacts/requests/'+fresh['id']+'/accept',{},b['token']);assert relationship(a,bid)['friend'] and relationship(b,aid)['friend']
 b_private=relationship(b,aid);new_open=call('POST','/api/contacts/'+bid+'/reopen',{'requestId':str(uuid.uuid4())},a['token']);observed=relationship(a,bid);assert observed['generation']==new_open['generation'] and observed['inboundReopened'];assert relationship(b,aid)==b_private
 sql("UPDATE directory.contacts SET request_generation=9007199254740993,request_gate='denied' WHERE owner_id='"+aid+"' AND peer_id='"+bid+"'")
 large=relationship(a,bid);assert large['generation']=='9007199254740993' and not large['inboundReopened'];assert relationship(b,aid)==b_private
 report={'status':'passed','httpAssertions':checks,'ownerOnlyMetadata':True,'noPeerPrivateGateProjection':True,'blockedByMeIsOwnerScoped':True,'queryOwnerOverrideIgnored':True,'defaultReadCreatesNoRows':True,'currentReopenGenerationOnly':True,'largeGenerationStaysDecimalString':True,'avatarACLUnchanged':True,'blockKeepsFriend':True,'newRoundsNeedNoReopen':True,'deleteRequiresNewAcceptance':True,'scope':'real HTTPS/two-account PostgreSQL relationship metadata; no full I acceptance'}
 destination=pathlib.Path(__file__).resolve().parents[3]/'项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '本人关系只读接口实测.json';destination.parent.mkdir(parents=True,exist_ok=True);destination.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
