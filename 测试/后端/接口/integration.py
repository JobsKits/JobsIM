#!/usr/bin/env python3
"""Real PostgreSQL/HTTP regression. Creates isolated named test accounts only."""
import argparse,concurrent.futures,http.cookiejar,json,secrets,ssl,urllib.request,urllib.error,uuid
p=argparse.ArgumentParser();p.add_argument('--server',required=True);p.add_argument('--ca-file');args=p.parse_args();base=args.server.rstrip('/');assert base.startswith(('http://127.0.0.1:','https://'))
checks=0;tokens=[];epoch=None;context=ssl.create_default_context(cafile=args.ca_file)
def call(method,path,body=None,token=None,expect=200,opener=None,headers=None):
 global checks
 if method=='POST' and '/conversations/' in path and path.endswith('/messages') and body is not None and epoch is not None:body=dict(body,restoreEpoch=body.get('restoreEpoch',epoch))
 data=None if body is None else json.dumps(body).encode();h={'Content-Type':'application/json'}
 if token:h['Authorization']='Bearer '+token
 if opener and getattr(opener,'auth_family',None):h['X-JobsIM-Auth-Family']=opener.auth_family
 if headers:h.update(headers)
 req=urllib.request.Request(base+path,data=data,method=method,headers=h)
 try:
  with (opener.open(req) if opener else urllib.request.urlopen(req,timeout=15,context=context)) as r:status=r.status;payload=r.read()
 except urllib.error.HTTPError as e:status=e.code;payload=e.read()
 assert status==expect,(method,path,status,'unexpected response redacted');checks+=1
 return json.loads(payload) if payload else None
suffix=secrets.token_hex(4);password='Integration-only strong phrase 924!'
def register(prefix,web=False,opener=None):
 result=call('POST','/api/auth/register',{'username':prefix+suffix,'password':password,'requestId':str(uuid.uuid4()),'deviceName':'integration'},headers={'X-JobsIM-Client':'web','Origin':base} if web else {},opener=opener)
 if result.get('token'):tokens.append(result['token'])
 if opener and result.get('familyId'):opener.auth_family=result['familyId']
 return result
try:
 boot=call('GET','/api/bootstrap');assert boot['deploymentId'] and not boot['features']['after_receipt'];epoch=boot['restoreEpoch']
 a=register('alice');b=register('bob');c=register('carol');at=a['token'];bt=b['token'];ct=c['token']
 call('GET','/api/me',token=at);call('GET','/api/me',expect=401)
 requesterName='Visible requester '+suffix;call('PATCH','/api/me',{'displayName':requesterName},at)
 call('DELETE','/api/auth/sessions/invalid-device-id',token=at,expect=400)
 call('PUT','/api/blocks/invalid-user-id',{},at,expect=400)
 contactRequestID=str(uuid.uuid4());request=call('POST','/api/contacts/requests',{'userId':b['user']['id'],'requestId':contactRequestID},at)
 inbox=call('GET','/api/contacts',token=bt);incoming=next(x for x in inbox['requests'] if x['id']==request['id']);assert incoming['requester']['id']==a['user']['id'] and incoming['requester']['username']==a['user']['username'] and incoming['requester']['displayName']==requesterName
 call('POST','/api/contacts/requests/'+request['id']+'/accept',{},bt)
 contacts=call('GET','/api/contacts',token=at);assert contacts['items'][0]['id']==b['user']['id']
 conv=call('POST','/api/conversations',{'kind':'direct','memberIds':[b['user']['id']]},at)
 again=call('POST','/api/conversations',{'kind':'direct','memberIds':[a['user']['id']]},bt);assert conv['id']==again['id']
 path='/api/conversations/'+conv['id']+'/messages';original={'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Durable hello 中文消息','sendAdmission':conv['sendAdmission']}
 m=call('POST',path,original,at);same=call('POST',path,original,at);assert same['id']==m['id']
 call('POST',path,dict(original,text='changed payload'),at,expect=409)
 call('GET',path,token=ct,expect=403)
 reply=call('POST',path,{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'A reply','replyToId':m['id'],'sendAdmission':again['sendAdmission']},bt);assert reply['replyToId']==m['id']
 call('PATCH','/api/messages/'+m['id'],{'text':'Edited durable 中文','expectedVersion':m['version']},at)
 call('PATCH','/api/messages/'+m['id'],{'text':'Stale edit','expectedVersion':m['version']},at,expect=409)
 call('POST','/api/messages/'+m['id']+'/reactions',{'emoji':'👍'},bt)
 call('PUT','/api/messages/'+m['id']+'/favorite',{},bt);call('PUT','/api/messages/'+m['id']+'/pin',{},at)
 found=call('GET','/api/search?q=durable',token=bt);assert any(x['id']==m['id'] for x in found['items'])
 history=call('GET',path,token=bt);assert len(history['items'])==2;assert history['cursor']!='0'
 call('PUT','/api/blocks/'+b['user']['id'],{},at);call('POST',path,{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Blocked','sendAdmission':again['sendAdmission']},bt,expect=409)
 call('POST',path,{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Blocked reverse','sendAdmission':conv['sendAdmission']},at,expect=409)
 for actor,peer in [(at,b['user']['id']),(bt,a['user']['id'])]:assert any(item['id']==peer for item in call('GET','/api/contacts',token=actor)['items'])
 blocked_inbox=call('GET','/api/contacts',token=bt);assert 'requester' not in next(x for x in blocked_inbox['requests'] if x['id']==request['id'])
 blocked=call('POST','/api/conversations',{'kind':'direct','memberIds':[b['user']['id']]},at);assert blocked['id']==conv['id'] and blocked['sendAdmission']==''
 call('DELETE','/api/blocks/'+b['user']['id'],token=at);call('POST','/api/contacts/requests/'+request['id']+'/accept',{},bt,expect=409)
 restored=call('POST','/api/conversations',{'kind':'direct','memberIds':[b['user']['id']]},at);assert restored['sendAdmission'] and restored['sendAdmission']!=conv['sendAdmission']
 call('POST',path,{'clientMsgId':str(uuid.uuid4()),'type':'text','text':'Old admission retired','sendAdmission':conv['sendAdmission']},at,expect=409)
 # Delete removes both lists; only a new independent request and acceptance rebuild it.
 call('DELETE','/api/contacts/'+b['user']['id'],token=at)
 for actor,peer in [(at,b['user']['id']),(bt,a['user']['id'])]:assert all(item['id']!=peer for item in call('GET','/api/contacts',token=actor)['items'])
 renewed=call('POST','/api/contacts/requests',{'userId':a['user']['id'],'requestId':str(uuid.uuid4())},bt);call('POST','/api/contacts/requests/'+renewed['id']+'/reject',{},at)
 next_request={'userId':a['user']['id'],'requestId':str(uuid.uuid4())};renewed=call('POST','/api/contacts/requests',next_request,bt);assert call('POST','/api/contacts/requests',next_request,bt)['id']==renewed['id']
 duplicate=call('POST','/api/contacts/requests',{'userId':a['user']['id'],'requestId':str(uuid.uuid4())},bt,expect=409);assert duplicate['error']['code']=='CONTACT_REQUEST_PENDING'
 call('POST','/api/contacts/requests/'+renewed['id']+'/accept',{},at)
 call('POST','/api/contacts/requests',{'userId':b['user']['id'],'requestId':contactRequestID},at,expect=409)
 # Group membership and author-only editing are independent of direct friendship state.
 selfconv=call('POST','/api/conversations',{'kind':'self','memberIds':[]},at);assert selfconv['memberIds']==[a['user']['id']]
 jar=http.cookiejar.CookieJar();opener=urllib.request.build_opener(urllib.request.HTTPSHandler(context=context),urllib.request.HTTPCookieProcessor(jar));web=register('web',True,opener);assert 'token' not in web
 call('GET','/api/me',opener=opener,expect=401)
 csrf=call('GET','/api/auth/csrf',opener=opener)['csrfToken'];call('POST','/api/auth/confirm',{},opener=opener,headers={'X-CSRF-Token':csrf,'Origin':base})
 call('GET','/api/me',opener=opener);call('POST','/api/conversations',{'kind':'self'},opener=opener,expect=403)
 call('POST','/api/auth/logout',{},opener=opener,headers={'X-CSRF-Token':csrf,'Origin':base});call('GET','/api/me',opener=opener,expect=401)
 # Recovery response loss must not reveal old codes and recovery revokes prior sessions.
 codes=call('POST','/api/auth/recovery-codes',{'password':password,'requestId':'codes-'+suffix},at);assert len(codes['codes'])==10
 retry=call('POST','/api/auth/recovery-codes',{'password':password,'requestId':'codes-'+suffix},at);assert retry['codes']==[]
 newpassword='New integration-only phrase 531!';recover={'username':'alice'+suffix,'code':codes['codes'][0],'password':newpassword,'requestId':'recover-'+suffix}
 call('POST','/api/auth/recover',recover);call('POST','/api/auth/recover',recover);call('GET','/api/me',token=at,expect=401)
 call('POST','/api/auth/recover',dict(recover,requestId='another-'+suffix),expect=403)
 print(json.dumps({'status':'passed','httpAssertions':checks,'deploymentId':boot['deploymentId'],'scope':'real PostgreSQL HTTP functional slice; no full I or recovery acceptance'}))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
