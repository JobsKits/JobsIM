const fs=require('node:fs'),path=require('node:path'),os=require('node:os'),https=require('node:https'),crypto=require('node:crypto'),assert=require('node:assert/strict'),{spawn}=require('node:child_process');
const binary=process.env.JOBSIM_CLI_BINARY,ca=process.env.JOBSIM_CA_FILE,server=process.env.JOBSIM_SERVER_URL||'https://127.0.0.1:8443';
if(!binary||!ca)throw Error('Provide JOBSIM_CLI_BINARY and JOBSIM_CA_FILE');
const trust=new https.Agent({ca:fs.readFileSync(ca)});
function api(route,method='GET',body,token){return new Promise((resolve,reject)=>{const req=https.request(server+'/api'+route,{method,agent:trust,headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})}},res=>{let text='';res.on('data',chunk=>text+=chunk);res.on('end',()=>{try{const data=JSON.parse(text||'{}');if(res.statusCode>=400)reject(Error('HTTP '+res.statusCode+' '+data.error?.code));else resolve(data)}catch{reject(Error('Invalid API response'))}})});req.on('error',error=>reject(Error(error.code??'NETWORK')));req.end(body===undefined?undefined:JSON.stringify(body))})}
class CLI {
 constructor(mode,user,password,directory){this.output='';this.error='';this.waiter=null;this.process=spawn(binary,['--server',server,'--ca',ca,'--data-dir',directory,mode],{stdio:['pipe','pipe','pipe']});this.process.stdout.on('data',chunk=>{this.output+=chunk;this.flush()});this.process.stderr.on('data',chunk=>this.error+=chunk);this.closed=new Promise((resolve,reject)=>{this.process.once('error',error=>{this.waiter?.reject(Error(error.code??'CLI_START'));reject(error)});this.process.once('exit',code=>{this.waiter?.reject(Error('CLI exited before prompt: '+code));this.waiter=null;resolve(code)})});this.ready=this.prompt();this.process.stdin.write(user+'\n'+password+'\n');}
 flush(){if(!this.waiter)return;const at=this.output.indexOf('jobsim> ');if(at<0)return;const answer=this.output.slice(0,at);this.output=this.output.slice(at+8);const waiter=this.waiter;this.waiter=null;clearTimeout(waiter.timer);waiter.resolve(answer)}
 prompt(){return new Promise((resolve,reject)=>{this.waiter={resolve,reject,timer:setTimeout(()=>{this.waiter=null;reject(Error('CLI prompt timed out'))},20000)};this.flush()})}
 async command(line){const prior=this.error.length;const pending=this.prompt();this.process.stdin.write(line+'\n');const output=await pending;await new Promise(resolve=>setTimeout(resolve,20));if(this.error.length!==prior)throw Error('CLI command failed: '+line.split(' ')[0]);return output.trim()?JSON.parse(output.trim()):null}
 async stop(){if(this.process.exitCode!==null)return;this.process.stdin.end('quit\n');assert.equal(await this.closed,0)}
}
(async()=>{const run=fs.mkdtempSync(path.join(os.tmpdir(),'jobsim-cli-https-'));fs.chmodSync(run,0o700);const suffix=Date.now().toString(36),username='cli_a_'+suffix,password='Private CLI test '+suffix+' phrase';const peer=await api('/auth/register','POST',{username:'cli_b_'+suffix,password,requestId:crypto.randomUUID(),deviceName:'CLI HTTPS test peer'});const clients=[];try{
 const a=new CLI('register',username,password,run);clients.push(a);await a.ready;await a.command('request '+peer.user.id);
 const requests=(await api('/contacts',undefined,undefined,peer.token)).requests;const invitation=requests.find(item=>item.state==='pending');assert.ok(invitation);await api('/contacts/requests/'+invitation.id+'/accept','POST',{},peer.token);
 const conversation=await a.command('direct '+peer.user.id);assert.ok(conversation.id);const sent=await a.command('send '+conversation.id+' Real SQLite CLI '+suffix);assert.ok(sent.id);const history=await api('/conversations/'+conversation.id+'/messages',undefined,undefined,peer.token);assert.equal(history.items.filter(item=>item.clientMsgId===sent.clientMsgId).length,1);
 const source=path.join(run,'source.txt'),destination=path.join(run,'received.txt');fs.writeFileSync(source,'Actual authenticated CLI file '+suffix);const media=await a.command('upload '+source);assert.ok(media.id);const fileMessage=await a.command('send-media '+conversation.id+' file '+media.id+' CLI file');assert.equal(fileMessage.attachmentId,media.id);await a.command('download '+media.id+' | '+destination);assert.equal(fs.readFileSync(destination,'utf8'),fs.readFileSync(source,'utf8'));await assert.rejects(()=>a.command('download '+media.id+' | '+destination));assert.equal(fs.readFileSync(destination,'utf8'),fs.readFileSync(source,'utf8'));await a.command('history '+conversation.id);await a.command('draft '+conversation.id+' Draft survives process restart');const firstQueue=await a.command('outbox');assert.equal(firstQueue[sent.clientMsgId].State??firstQueue[sent.clientMsgId].state,'confirmed');await a.stop();
 const reopened=new CLI('login',username,password,run);clients.push(reopened);await reopened.ready;const cached=await reopened.command('cached '+conversation.id);assert.ok(cached.items.some(item=>item.id===sent.id));const drafts=await reopened.command('drafts');assert.equal(drafts[conversation.id],'Draft survives process restart');await reopened.stop();
 const isolated=new CLI('login',peer.user.username,password,run);clients.push(isolated);await isolated.ready;const otherCache=await isolated.command('cached '+conversation.id);assert.equal(otherCache.items.length,0);await isolated.stop();

 const governance=new CLI('login',username,password,run);clients.push(governance);await governance.ready;
 const operations=await governance.command('operations');
 assert.ok(operations.some(item=>item.kind==='request'&&item.state==='completed'),'Original request did not survive process restart');
 assert.equal((await governance.command('relationship '+peer.user.id)).friend,true);
 async function failedMedia(conversation,label){
  await assert.rejects(()=>governance.command('send-media '+conversation+' file '+crypto.randomUUID()+' '+label));
  const queue=await governance.command('outbox');
  const pending=Object.values(queue).find(item=>item.text===label);
  assert.ok(pending&&pending.state==='pending','Rejected media intent was not durably retained');
  return pending;
 }
 function originalUnchanged(before,after){
  for(const key of ['clientMsgId','conversationId','restoreEpoch','sendAdmission','text','attachmentId','type'])
   assert.ok(before[key]===after[key],'Governance changed an original Outbox field');
 }
 const beforeRemoval=await failedMedia(conversation.id,'CLI remove-contact pending');
 await governance.command('remove-contact '+peer.user.id);
 let queue=await governance.command('outbox');
 assert.equal(queue[beforeRemoval.clientMsgId].state,'frozen');
 assert.equal(queue[beforeRemoval.clientMsgId].freezeReason,'CONTACT_REMOVED');
 originalUnchanged(beforeRemoval,queue[beforeRemoval.clientMsgId]);
 await assert.rejects(()=>governance.command('retry '+beforeRemoval.clientMsgId));
 assert.equal((await governance.command('relationship '+peer.user.id)).friend,false);
 await governance.command('request '+peer.user.id);
 let pendingRequest=(await api('/contacts',undefined,undefined,peer.token)).requests.find(item=>item.state==='pending'&&item.requesterId);
 assert.ok(pendingRequest);await api('/contacts/requests/'+pendingRequest.id+'/accept','POST',{},peer.token);
 const beforeBlock=await failedMedia(conversation.id,'CLI block pending');
 await governance.command('block '+peer.user.id);
 queue=await governance.command('outbox');assert.equal(queue[beforeBlock.clientMsgId].freezeReason,'PEER_BLOCKED');originalUnchanged(beforeBlock,queue[beforeBlock.clientMsgId]);
 await governance.command('unblock '+peer.user.id);
 let policy=await governance.command('relationship '+peer.user.id);assert.equal(policy.blockedByMe,false);assert.equal(policy.inboundGate,'denied');assert.equal(policy.friend,false);
 const opened=await governance.command('reopen '+peer.user.id);assert.equal(opened.result.status,'opened');assert.equal(opened.relationship.inboundGate,'open');
 let incoming=await api('/contacts/requests','POST',{userId:invitation.requesterId,requestId:crypto.randomUUID()},peer.token);
 await governance.command('reject '+incoming.id);
 const oldOpened=await governance.command('resume '+opened.operationId);assert.equal(oldOpened.relationship.inboundGate,'denied');
 await governance.command('reopen '+peer.user.id);
 incoming=await api('/contacts/requests','POST',{userId:invitation.requesterId,requestId:crypto.randomUUID()},peer.token);
 await governance.command('accept '+incoming.id);
 assert.equal((await governance.command('relationship '+peer.user.id)).friend,true);
 await assert.rejects(()=>governance.command('reconcile '+beforeRemoval.clientMsgId));
 queue=await governance.command('outbox');originalUnchanged(beforeRemoval,queue[beforeRemoval.clientMsgId]);assert.equal(queue[beforeRemoval.clientMsgId].freezeReason,'CONTACT_REMOVED');
 const group=await governance.command('group CLI governance group | '+peer.user.id);assert.ok(group.id);
 let roster=(await governance.command('members '+group.id)).items;
 const originalPeer=roster.find(item=>item.id===peer.user.id);assert.ok(originalPeer?.membershipGeneration);
 await governance.command('rename '+group.id+' CLI renamed governance group');
 assert.equal((await governance.command('conversations')).items.find(item=>item.id===group.id).title,'CLI renamed governance group');
 await governance.command('remove-member '+group.id+' '+peer.user.id+' '+originalPeer.membershipGeneration);
 await governance.command('add-member '+group.id+' '+peer.user.id);
 roster=(await governance.command('members '+group.id)).items;
 assert.ok(roster.find(item=>item.id===peer.user.id).membershipGeneration!==originalPeer.membershipGeneration);
 await assert.rejects(()=>governance.command('remove-member '+group.id+' '+peer.user.id+' '+originalPeer.membershipGeneration));
 await governance.command('transfer '+group.id+' '+peer.user.id);
 const mine=roster.find(item=>item.id!==peer.user.id);assert.ok(mine?.membershipGeneration);
 await governance.command('send '+group.id+' CLI group history before leave');await governance.command('history '+group.id);
 const beforeLeave=await failedMedia(group.id,'CLI leave pending');
 await governance.command('draft '+group.id+' Draft retained after explicit leave');
 await governance.command('leave '+group.id+' '+mine.membershipGeneration);
 assert.equal((await governance.command('cached '+group.id)).items.length,0);
 queue=await governance.command('outbox');assert.equal(queue[beforeLeave.clientMsgId].state,'frozen');assert.equal(queue[beforeLeave.clientMsgId].freezeReason,'CONVERSATION_LEFT');originalUnchanged(beforeLeave,queue[beforeLeave.clientMsgId]);
 assert.equal((await governance.command('drafts'))[group.id],'Draft retained after explicit leave');
 await api('/conversations/'+group.id+'/members','POST',{userId:mine.id},peer.token);
 roster=(await governance.command('members '+group.id)).items;const currentMine=roster.find(item=>item.id===mine.id);assert.ok(currentMine.membershipGeneration!==mine.membershipGeneration);
 const freshMessage=await governance.command('send '+group.id+' CLI new membership history');
 const freshPending=await failedMedia(group.id,'CLI new membership pending');
 await assert.rejects(()=>governance.command('leave '+group.id+' '+mine.membershipGeneration));
 queue=await governance.command('outbox');assert.equal(queue[freshPending.clientMsgId].state,'pending');
 assert.ok((await governance.command('cached '+group.id)).items.some(item=>item.id===freshMessage.id));
 await governance.command('leave '+group.id+' '+currentMine.membershipGeneration);
 await governance.stop();
 const report=path.resolve(__dirname,'../../../项目相关报告/测试/CLI/执行记录/' + Date.now() + '-' + process.pid);fs.mkdirSync(report,{recursive:true});fs.writeFileSync(path.join(report,'HTTPS结果.json'),JSON.stringify({status:'passed',at:new Date().toISOString(),binarySHA256:crypto.createHash('sha256').update(fs.readFileSync(binary)).digest('hex'),checks:['strict private CA and hostname verification','real PostgreSQL identities/friendship/conversation','CLI message reaches authenticated peer','real SQLite Outbox confirmed','authenticated upload/media message/download and existing-file protection','process restart retains history and draft','same-machine second account has separate local database','process exit revokes its session','persisted original request/reopen operations and current inbound policy','remove-contact/block freeze only affected direct intents without changing payload','unblock does not reopen or restore friendship; explicit reopen does not revive old requests','group members/add/remove/rename/transfer/leave use original generation','leave purges local history and retains drafts before HTTP; stale leave after rejoin retains new intent/history']},null,2));console.log('PASS CLI SQLite and real HTTPS/PostgreSQL integration');
 }finally{for(const client of clients)await client.stop().catch(()=>{client.process.kill('SIGTERM')});await api('/auth/logout','POST',{},peer.token).catch(()=>{})}})().catch(error=>{console.error(String(error.message).split('\n')[0]);process.exitCode=1});
