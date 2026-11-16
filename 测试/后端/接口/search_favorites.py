#!/usr/bin/env python3
"""Real HTTPS regression: qualify before pagination, owner-only favorites."""
import argparse,json,pathlib,secrets,ssl,urllib.error,urllib.request,uuid

p=argparse.ArgumentParser();p.add_argument('--environment-file',required=True);p.add_argument('--report',type=pathlib.Path);args=p.parse_args();env=json.loads(pathlib.Path(args.environment_file).read_text());base=env['serverURL'];trust=ssl.create_default_context(cafile=env['caPath']);checks=0;tokens=[]
def call(method,path,body=None,token=None,expect=200):
 global checks
 request=urllib.request.Request(base+path,method=method,data=None if body is None else json.dumps(body).encode(),headers={'Content-Type':'application/json',**({'Authorization':'Bearer '+token} if token else{})})
 try:
  with urllib.request.urlopen(request,context=trust,timeout=20) as response:status=response.status;data=response.read()
 except urllib.error.HTTPError as error:status=error.code;data=error.read()
 assert status==expect,(method,path,status,'response redacted');checks+=1;return json.loads(data) if data else None
def register(prefix):
 value=call('POST','/api/auth/register',{'username':prefix+secrets.token_hex(4),'password':'Search fixture phrase '+secrets.token_hex(12),'requestId':str(uuid.uuid4()),'deviceName':'search-favorites-regression'});tokens.append(value['token']);return value
def create(token,kind='self',members=None):return call('POST','/api/conversations',{'kind':kind,'title':'Scoped search fixture','memberIds':members or[]},token)
def send(session,conversation,text):return call('POST','/api/conversations/'+conversation['id']+'/messages',{'clientMsgId':str(uuid.uuid4()),'type':'text','text':text,'restoreEpoch':epoch,'sendAdmission':conversation['sendAdmission']},session['token'])
def pages(token,path,limit):
 items=[];cursor='0';seen=set()
 for _ in range(100):
  value=call('GET',path+('&' if '?' in path else '?')+'limit='+str(limit)+'&cursor='+cursor,token=token)
  assert isinstance(value['cursor'],str) and isinstance(value['hasMore'],bool)
  for item in value['items']:assert item['id'] not in seen;seen.add(item['id']);items.append(item)
  if not value['hasMore']:return items
  assert value['items'] and value['cursor']!=cursor;cursor=value['cursor']
 raise AssertionError('Pagination did not terminate')
try:
 epoch=call('GET','/api/bootstrap')['restoreEpoch'];a,b=register('search_a_'),register('search_b_');at,bt=a['token'],b['token'];bid=b['user']['id'];term='scope'+secrets.token_hex(6)
 ac,bc=create(at),create(bt);own=[send(a,ac,term+' owned '+str(i)) for i in range(8)]
 # More recent foreign hits must never occupy this owner's SQL result budget.
 for i in range(110):send(b,bc,term+' foreign '+str(i))
 result=pages(at,'/api/search?q='+term,3);assert {m['id'] for m in result}=={m['id'] for m in own};assert all(m['senderId']==a['user']['id'] for m in result)
 first=call('GET','/api/search?q='+term+'&limit=3',token=at);assert first['hasMore'] and len(first['items'])==3
 call('GET','/api/search?q='+term+'&limit=3&cursor='+first['cursor'],token=bt,expect=400)
 call('GET','/api/search?q=other&cursor='+first['cursor'],token=at,expect=400)
 call('GET','/api/search?q='+term+'&cursor='+first['cursor']+'x',token=at,expect=400)
 assert call('GET','/api/search?q='+term+'&conversationId='+bc['id'],token=at)['items']==[]
 call('GET','/api/search?q=',token=at,expect=400);call('GET','/api/search?q=%FF',token=at,expect=400);call('GET','/api/favorites',expect=401);call('GET','/api/favorites?limit=0',token=at,expect=400)
 assert call('GET','/api/favorites',token=at)['items']==[]
 for message in own[:5]:call('PUT','/api/messages/'+message['id']+'/favorite',{},at)
 favorites=pages(at,'/api/favorites',2);assert {m['id'] for m in favorites}=={m['id'] for m in own[:5]} and all(m['favorite'] for m in favorites)
 favorite_cursor=call('GET','/api/favorites?limit=2',token=at)['cursor'];call('GET','/api/search?q='+term+'&cursor='+favorite_cursor,token=at,expect=400)
 assert call('GET','/api/favorites',token=bt)['items']==[]
 call('DELETE','/api/messages/'+own[0]['id'],token=at);assert own[0]['id'] not in {m['id'] for m in pages(at,'/api/favorites',2)}
 retracted=call('GET','/api/messages/'+own[0]['id'],token=at);assert retracted['deleted'] and retracted['text']==''
 call('GET','/api/messages/'+own[0]['id'],token=bt,expect=403)
 call('GET','/api/messages/invalid-id',token=at,expect=400);call('GET','/api/conversations/invalid-id',token=at,expect=400)
 call('DELETE','/api/messages/'+own[1]['id']+'/favorite',token=at);assert own[1]['id'] not in {m['id'] for m in pages(at,'/api/favorites',2)}
 request=call('POST','/api/contacts/requests',{'userId':bid,'requestId':str(uuid.uuid4())},at);call('POST','/api/contacts/requests/'+request['id']+'/accept',{},bt)
 group=create(at,'group',[bid]);source=send(a,group,term+' shared group source');call('PUT','/api/messages/'+source['id']+'/favorite',{},bt)
 assert source['id'] in {m['id'] for m in pages(bt,'/api/favorites',2)}
 member=next(m for m in call('GET','/api/conversations/'+group['id']+'/members',token=at)['items'] if m['id']==bid)
 call('DELETE','/api/conversations/'+group['id']+'/members/'+bid+'?membershipGeneration='+member['membershipGeneration'],token=at);assert call('GET','/api/favorites',token=bt)['items']==[]
 call('POST','/api/conversations/'+group['id']+'/members',{'userId':bid},at);assert call('GET','/api/favorites',token=bt)['items']==[]
 # Search's authority input is not the public conversation screen's top 100.
 hidden=create(at,'group');deep='deep'+secrets.token_hex(6);hidden_message=send(a,hidden,deep)
 for _ in range(101):create(at,'group')
 assert hidden['id'] not in {c['id'] for c in call('GET','/api/conversations',token=at)['items']}
 assert [m['id'] for m in pages(at,'/api/search?q='+deep,2)]==[hidden_message['id']]
 located=call('GET','/api/conversations/'+hidden['id'],token=at);assert located['id']==hidden['id'] and located['sendAdmission'] and located['membershipGeneration']=='1'
 assert call('GET','/api/messages/'+hidden_message['id'],token=at)['id']==hidden_message['id']
 call('GET','/api/conversations/'+hidden['id'],token=bt,expect=403);call('GET','/api/messages/'+hidden_message['id'],token=bt,expect=403)
 call('GET','/api/conversations/'+hidden['id'],expect=401);call('GET','/api/messages/'+hidden_message['id'],expect=401)
 report={'status':'passed','httpAssertions':checks,'foreignRecentHits':110,'ownedConversationsBeyondPublicTop100':True,'qualifyBeforeSQLLimit':True,'stableOwnerEpochQueryBoundCursor':True,'favoritesNeedNoSearchQuery':True,'favoriteSourceACLAndRejoinFloor':True,'retractedAndUnfavoritedSourcesOmitted':True,'exactSourceLookupRechecksReadACL':True,'strictHTTPS':True,'scope':'functional PostgreSQL substring search and source references; no full-text performance or complete I claim'}
 target=args.report or pathlib.Path(__file__).resolve().parents[3]/'项目相关报告/测试/后端/执行记录' / uuid.uuid4().hex / '搜索与本人收藏分页实测.json';target.parent.mkdir(parents=True,exist_ok=True);target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n');print(json.dumps(report))
finally:
 for token in tokens:
  try:call('POST','/api/auth/logout',{},token)
  except Exception:pass
