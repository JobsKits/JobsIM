import test from 'node:test';
import assert from 'node:assert/strict';
import {createRequire} from 'node:module';
const require=createRequire(import.meta.url);
const {Rules,Isolation}=require(process.env.JOBS_IM_HARMONY_HOST_MODELS);
const {Strings}=require(process.env.JOBS_IM_HARMONY_HOST_STRINGS);
test('pending inbox is recipient-bound and never resurrects accepted requests',()=>{
  const r={id:'request',requesterId:'sender',recipientId:'recipient',state:'pending'};
  assert.equal(Rules.incoming(r,'recipient'),true);assert.equal(Rules.incoming(r,'sender'),false);
  for(const state of ['accepted','rejected','expired','cancelled'])assert.equal(Rules.incoming({...r,state},'recipient'),false);
});
test('read-only and unavailable-owner groups reject writes',()=>{
  assert.equal(Rules.writable({readOnly:true}),false);
  assert.equal(Rules.writable({readOnly:false,state:'owner_unavailable'}),false);
  assert.equal(Rules.writable({state:'tombstoned'}),false);
  assert.equal(Rules.writable({readOnly:false,state:'active'}),true);
});
test('original group membership generation remains decimal text above JS safe integer',()=>{
  assert.equal(Rules.removalPath('conversation',{id:'peer',membershipGeneration:'9007199254740993'}),'/conversations/conversation/members/peer?membershipGeneration=9007199254740993');
  for(const membershipGeneration of ['','0','1.5','-1','unknown'])assert.throws(()=>Rules.removalPath('c',{id:'p',membershipGeneration}));
});
test('profile, account, device, demo and restore epochs each isolate local data',()=>{
  const base=['profile','deployment','account',false,'2','device'];
  const original=Isolation.namespace(...base);
  for(const [index,value] of [[0,'other'],[1,'other'],[2,'other'],[3,true],[4,'3'],[5,'other']]){
    const changed=[...base];changed[index]=value;assert.notEqual(Isolation.namespace(...changed),original);
  }
});
test('opaque history cursor is epoch-bound without integer coercion',()=>{
  assert.equal(Rules.cursorMatches('3','3:900719925474099312345'),true);
  for(const c of ['2:900719925474099312345','3:-1','3:1.2','0','NaN','3:'])assert.equal(Rules.cursorMatches('3',c),false);
});
test('restored secure credentials require matching account, device and epoch',()=>{
  const p={account:'a',device:'d',epoch:'2'};const c={user:{id:'a'},deviceId:'d',restoreEpoch:'2'};
  assert.equal(Rules.sessionMatches(p,c),true);
  assert.equal(Rules.sessionMatches(p,{...c,user:{id:'other'}}),false);
  assert.equal(Rules.sessionMatches(p,{...c,deviceId:'other'}),false);
  assert.equal(Rules.sessionMatches(p,{...c,restoreEpoch:'3'}),false);
});
test('strict authentication JSON separates login OTP from recovery code',()=>{
  const login=JSON.parse(Rules.authBody('login','alice','fixture-password-long','123456','request-fixture'));
  const recover=JSON.parse(Rules.authBody('recover','alice','fixture-password-long','one-use-fixture','request-fixture'));
  assert.deepEqual(Object.keys(login).sort(),['username','password','requestId','deviceName','otp'].sort());
  assert.deepEqual(Object.keys(recover).sort(),['username','password','code','requestId'].sort());
  assert.equal(login.otp,'123456');assert.equal(recover.code,'one-use-fixture');
});
test('connection descriptions reject credentials, HTTP, queries and fragments',()=>{
  assert.equal(Isolation.endpoint('https://chat.example.test/'),'https://chat.example.test');
  for(const u of ['http://chat.example.test','https://user:password@chat.example.test','https://chat.example.test?token=x','https://chat.example.test#x'])assert.throws(()=>Isolation.endpoint(u));
});
test('all ten language tables cover the current control vocabulary',()=>{
  assert.equal(Strings.languages.length,10);assert.equal(new Set(Strings.keys).size,Strings.keys.length);
  assert.equal(Strings.rows.length,10);
  for(const row of Strings.rows){assert.equal(row.length,Strings.keys.length);assert.ok(row.every(x=>typeof x==='string'&&x.length>0));}
});
test('literal UI actions and labels resolve in every language',async()=>{
  const {readFile}=await import('node:fs/promises');
  const source=await readFile(process.env.JOBS_IM_HARMONY_UI_SOURCE,'utf8');
  const keys=[...source.matchAll(/\.(?:t|action)\('([^']+)'/g)].map(match=>match[1]);
  for(const key of keys)assert.ok(Strings.keys.includes(key),'Missing native UI key: '+key);
});

test('sending qualification support requires the current instance and an explicit capability',()=>{
  assert.equal(Rules.admissionSupported({deploymentId:'instance',features:{send_admission:true}},'instance'),true);
  for(const boot of [{deploymentId:'instance'},{deploymentId:'other',features:{send_admission:true}},{deploymentId:'instance',features:{send_admission:false}}])assert.equal(Rules.admissionSupported(boot,'instance'),false);
});
test('retries serialize the original admission and never infer missing legacy qualification',()=>{
  const row={clientMsgId:'original',restoreEpoch:'2',type:'text',text:'old intent',sendAdmission:'old-token'};
  const before=JSON.stringify(row);
  assert.equal(JSON.parse(Rules.outgoingBody(row)).sendAdmission,'old-token');
  assert.equal(JSON.stringify(row),before);
  const legacy={...row};delete legacy.sendAdmission;
  assert.equal(Object.hasOwn(JSON.parse(Rules.outgoingBody(legacy)),'sendAdmission'),false);
});

test('lookup paths keep search criteria and signed cursor separate, and favorites never use empty search',()=>{
  assert.equal(Rules.lookupPath('favorites','','',''),'/'+'favorites?limit=50');
  const path=Rules.lookupPath('search','a&b','group/id','signed+cursor=');
  const url=new URL('https://example.test'+path);
  assert.equal(url.pathname,'/search');assert.equal(url.searchParams.get('q'),'a&b');assert.equal(url.searchParams.get('conversationId'),'group/id');assert.equal(url.searchParams.get('cursor'),'signed+cursor=');
  assert.throws(()=>Rules.lookupPath('search','   ','',''));assert.throws(()=>Rules.lookupPath('unknown','','',''));
});

test('structured mentions require current members and owner-only all, with a copied stable selection',()=>{
  const group={id:'g',kind:'group',ownerId:'owner',state:'active',memberIds:['owner','peer'],cursor:'0'};
  const members=[{id:'peer',membershipGeneration:'9007199254740993'}];
  const chosen=['peer'];const frozen=Rules.selectedMentions(chosen,false,members,group,'owner');chosen.length=0;
  assert.deepEqual(frozen,['peer']);assert.deepEqual(Rules.selectedMentions([],true,members,group,'owner'),[]);
  for(const [ids,all,account] of [[['missing'],false,'owner'],[['peer','peer'],false,'owner'],[[],true,'peer'],[['peer'],true,'owner']])assert.throws(()=>Rules.selectedMentions(ids,all,members,group,account));
});
test('original outgoing mention IDs and all fields survive replay without a latest context rewrite',()=>{
  const row={clientMsgId:'same-original',restoreEpoch:'1',type:'text',text:'hello',sendAdmission:'original-token',mentionedUserIds:['peer']};
  const body=Rules.outgoingBody(row);assert.deepEqual(JSON.parse(body).mentionedUserIds,['peer']);assert.equal(JSON.parse(body).sendAdmission,'original-token');
  assert.equal(Rules.outgoingBody({...row,mentionAll:true,mentionedUserIds:undefined}),JSON.stringify({...JSON.parse(body),mentionedUserIds:undefined,mentionAll:true}));
  assert.equal(Object.hasOwn(JSON.parse(Rules.outgoingBody({...row,mentionedUserIds:undefined})),'mentionedUserIds'),false);
  assert.equal(Object.hasOwn(JSON.parse(body),'contentVersion'),false);
});
test('mention receipts do not re-alert on edit, read, block projection or later unblock',()=>{
  const message={id:'original',senderId:'peer',mentionedMe:true,deleted:false};
  assert.equal(Rules.mentionUnread(undefined,message,'me',false),true);
  assert.equal(Rules.mentionUnread(true,{...message,version:'2'},'me',false),true);
  assert.equal(Rules.mentionUnread(false,{...message,version:'2'},'me',false),false);
  assert.equal(Rules.mentionUnread(true,message,'me',true),false);
  assert.equal(Rules.mentionUnread(true,{...message,mentionedMe:false},'me',false),false);
  assert.equal(Rules.mentionUnread(false,message,'me',false),false);
  for(const changed of [{...message,deleted:true},{...message,contentUnavailable:true},{...message,senderId:'me'},{...message,mentionedMe:undefined}])assert.equal(Rules.mentionUnread(undefined,changed,'me',false),false);
});
test('announcement references content version rather than interaction version and rejects unavailable bodies',()=>{
  const source={id:'m',conversationId:'g',version:'9007199254740993',contentVersion:'2',deleted:false,text:'private source'};
  assert.equal(Rules.sourceContentVersion(source),'2');assert.equal(Rules.sourceContentVersion({...source,contentVersion:undefined}),'9007199254740993');
  for(const changed of [{...source,deleted:true},{...source,contentUnavailable:true},{...source,contentVersion:'0'},{...source,contentVersion:'1.5'}])assert.throws(()=>Rules.sourceContentVersion(changed));
});
test('persistent announcement intent contains no copied text, attachment or credentials',()=>{
  const group={id:'g',kind:'group',roleGeneration:'9007199254740995',membershipGeneration:'9007199254740993',state:'active'};
  const source={id:'m',conversationId:'g',version:'8',contentVersion:'2',text:'source body never stored in operation',attachmentId:'private-file',deleted:false};
  const intent=Rules.announcementIntent(group,source,'9007199254740994','request-original','2026-11-16T00:00:00.000Z');
  const body=JSON.parse(intent.body);assert.equal(intent.method,'PUT');assert.equal(body.sourceVersion,'2');assert.equal(body.membershipGeneration,'9007199254740993');
  assert.deepEqual(Object.keys(body).sort(),['expectedVersion','membershipGeneration','roleGeneration','requestId','messageId','sourceVersion','expiresAt'].sort());
  source.text='new text';source.contentVersion='3';group.membershipGeneration='9';assert.equal(JSON.parse(intent.body).sourceVersion,'2');assert.ok(!JSON.stringify(intent).includes('source body'));
  assert.throws(()=>Rules.announcementIntent({...group,kind:'direct'},source,'0','request-original'));
  assert.throws(()=>Rules.announcementIntent(group,{...source,conversationId:'other'},'0','request-original'));
});
test('announcement clear keeps only the original CAS, generation and request identity',()=>{
  const group={id:'g',kind:'group',roleGeneration:'9007199254740995',membershipGeneration:'3',state:'active'};
  const intent=Rules.announcementIntent(group,undefined,'2','clear-original');
  assert.equal(intent.method,'DELETE');assert.deepEqual(JSON.parse(intent.body),{expectedVersion:'2',membershipGeneration:'3',requestId:'clear-original',roleGeneration:'9007199254740995'});
  assert.throws(()=>Rules.announcementIntent({...group,readOnly:true},undefined,'2','clear-original'));
  assert.throws(()=>Rules.announcementIntent(group,undefined,'02','clear-original'));
});
test('announcement expiration is an explicit stable native preset',()=>{
  const now=Date.parse('2026-11-15T00:00:00.000Z');assert.equal(Rules.announcementExpiry('never',now),'');
  for(const [preset,seconds] of [['hour',3600],['day',86400],['week',604800]])assert.equal(Date.parse(Rules.announcementExpiry(preset,now))-now,seconds*1000);
  assert.throws(()=>Rules.announcementExpiry('unknown',now));
});
test('only a definite server rejection permits discarding an announcement intent',()=>{
  for(const code of ['ANNOUNCEMENT_VERSION_CONFLICT','MEMBERSHIP_GENERATION_CHANGED','ANNOUNCEMENT_SOURCE_CHANGED','ANNOUNCEMENT_FORBIDDEN','INVALID_EXPIRY','FEATURE_DISABLED'])assert.equal(Rules.announcementRejected('HTTP 409 '+code),true);
  for(const reason of ['timeout','HTTP 500 INTERNAL','HTTP 401','HTTP 404','HTTP 503 ADMISSION_RECOVERY_REQUIRED'])assert.equal(Rules.announcementRejected(reason),false);
});

test('privileged writes require explicit current-instance role admission support',()=>{
  assert.equal(Rules.roleAdmissionSupported({deploymentId:'i',features:{role_admission:true}},'i'),true);
  for(const boot of [{deploymentId:'i',features:{send_admission:true}},{deploymentId:'other',features:{role_admission:true}},{deploymentId:'i',features:{role_admission:false}}])assert.equal(Rules.roleAdmissionSupported(boot,'i'),false);
});
test('announcement role qualification is frozen and legacy bodies are never supplemented',()=>{
  const group={id:'g',kind:'group',state:'active',membershipGeneration:'4',roleGeneration:'7'};
  const intent=Rules.announcementIntent(group,undefined,'0','original-operation');group.roleGeneration='9';
  assert.equal(JSON.parse(intent.body).roleGeneration,'7');
  assert.throws(()=>Rules.announcementIntent({...group,roleGeneration:undefined},undefined,'0','new-operation'));
  const legacy={conversationId:'g',method:'DELETE',body:JSON.stringify({expectedVersion:'0',membershipGeneration:'4',requestId:'legacy-operation'})};
  assert.equal(Object.hasOwn(JSON.parse(legacy.body),'roleGeneration'),false);
  assert.equal(Rules.announcementRejected('HTTP 409 QUALIFICATION_CHANGED'),true);
  assert.equal(Rules.permanentMentionRejection('HTTP 403 MENTION_ALL_FORBIDDEN'),true);
  assert.equal(Rules.permanentMentionRejection('HTTP 429 MENTION_RATE_LIMIT'),false);
});

test('forward intents freeze only source references and never copy source content',()=>{
 const source={id:'source',conversationId:'origin',text:'private body',attachmentId:'private-media',replyToId:'reply',contentVersion:'9007199254740997',forwardAdmission:'original-source-proof'};
 const target={id:'destination',state:'active',sendAdmission:'original-target-proof'};
 const intent=Rules.forwardIntent(source,target,'actor','3','original-request','2026-11-15T00:00:00Z');
 const before=Rules.outgoingBody(intent); intent.sendAttempted=true;assert.equal(Rules.outgoingBody(intent),before);
 const body=JSON.parse(before);assert.equal(body.forwardSourceVersion,'9007199254740997');assert.equal(body.forwardAdmission,'original-source-proof');assert.equal(body.sendAdmission,'original-target-proof');
 for(const key of ['text','replyToId','attachmentId','mentionedUserIds','mentionAll'])assert.equal(Object.hasOwn(body,key),false);
 for(const invalid of [{...source,forwardAdmission:undefined},{...source,contentVersion:undefined},{...source,forwardUnavailable:true},{...source,deleted:true}])assert.throws(()=>Rules.forwardIntent(invalid,target,'actor','3','id','now'));
});
test('unavailable forward projection removes body and any source media authority',()=>{
 const cached={id:'forward-id',forwarded:true,text:'private',attachmentId:'original-media',attachmentURL:'/api/messages/forward-id/attachment',forwardAdmission:'transient',deleted:false};
 assert.equal(Rules.forwardPath(cached),'/messages/forward-id/attachment');
 assert.equal(Rules.forwardPath({...cached,attachmentURL:'/api/media/original-media'}),'');
 assert.equal(Rules.forwardPath({...cached,attachmentURL:'https://other.example/media'}),'');
 const masked=Rules.unavailableForward(cached);for(const key of ['text','attachmentId','attachmentURL','forwardAdmission'])assert.ok(!masked[key]);assert.equal(masked.forwardUnavailable,true);assert.equal(Rules.forwardPath(masked),'');
 for(const code of ['NOT_A_MEMBER','HISTORY_NOT_ALLOWED','INVALID_FORWARD','FORWARD_ADMISSION_CHANGED','FORWARD_SOURCE_CHANGED','FORWARD_SOURCE_UNAVAILABLE','FORWARD_DEPTH_LIMIT'])assert.equal(Rules.permanentForwardRejection(code),true);
 for(const code of ['HTTP 503 FORWARD_QUALIFICATION_UNAVAILABLE','HTTP 503 FORWARD_CLEANUP_PENDING','HTTP 429','timeout'])assert.equal(Rules.permanentForwardRejection(code),false);
});
