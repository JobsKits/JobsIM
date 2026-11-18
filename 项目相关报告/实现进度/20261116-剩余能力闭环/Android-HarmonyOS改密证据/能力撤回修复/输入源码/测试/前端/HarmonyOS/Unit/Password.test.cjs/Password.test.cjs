const test = require('node:test'), assert = require('node:assert/strict'), crypto = require('node:crypto'), path = require('node:path');
const work = process.env.JOBS_IM_PASSWORD_HOST;
const { PasswordChange, PasswordRules } = require(path.join(work, 'PasswordChange.js'));
const { PasswordText } = require(path.join(work, 'PasswordText.js'));
const { ApiFailure } = require(path.join(work, 'Models.js'));
const uuid = () => crypto.randomUUID();
const proof = () => PasswordRules.normalized('original-password-for-fixture', 'replacement-password-for-fixture', 'replacement-password-for-fixture', '123456');
function fixture() {
 const rows = [], calls = [], stops = []; let active = true, enabled = true, committed = false, lose = false, failDisk = false, state = 'changed';
 let owner = { user: uuid(), username: 'fixture_user', device: uuid(), family: uuid(), namespace: uuid(), token: 'synthetic-not-a-real-credential' };
 const ports = { profile: uuid(), origin: 'https://fixture.invalid', deployment: uuid(), epoch: '2', current: () => active, owner: () => owner, uuid,
  repository: { list: async () => structuredClone(rows), insert: async row => { if (failDisk) throw Error('disk fixture'); assert.equal(rows.length, 0); rows.push(structuredClone(row)); }, replace: async (before, after) => { const at = rows.findIndex(r => r.id === before.id); assert.deepEqual(rows[at], before); rows[at] = structuredClone(after); } },
  bootstrap: async () => ({ deploymentId: ports.deployment, restoreEpoch: '2', features: { own_password_change: enabled } }), stop: async row => { if (row.family === owner?.family) stops.push(row.family); },
  request: async (route, body, token) => { calls.push({ route, body: JSON.parse(body), token }); const row = rows[0];
   if (route.endsWith('-results') && !committed) throw new ApiFailure(404, 'PASSWORD_CHANGE_OPERATION_NOT_FOUND');
   if (route.endsWith('/password-change')) { assert.equal(row.state, 'unknown'); assert.ok(row.attempts > 0); committed = true; if (lose) throw new ApiFailure(0, 'TRANSPORT_UNKNOWN'); }
   return JSON.stringify({ requestId: row.id, status: state, restoreEpoch: row.epoch, credentialGeneration: '9007199254740993', securityVersion: '3', current: state === 'changed', requiresNewLogin: state === 'changed' });
  }
 };
 return { service: new PasswordChange(ports), ports, rows, calls, stops, set lose(v) { lose = v; }, set enabled(v) { enabled = v; }, set active(v) { active = v; }, set failDisk(v) { failDisk = v; }, set state(v) { state = v; }, set owner(v) { owner = v; }, get owner() { return owner; }, set committed(v) { committed = v; } };
}
test('NFC, codepoint bounds, password reuse and confirmation are checked', () => {
 assert.equal(PasswordRules.normalized('original-password', 'abcdefghijklmn' + 'e\u0301', 'abcdefghijklmn\u00e9', '').newPassword, 'abcdefghijklmn\u00e9');
 for (const next of ['short', 'x'.repeat(129), 'original-password']) assert.throws(() => PasswordRules.normalized('original-password', next, next, ''));
 assert.throws(() => PasswordRules.normalized('original-password', 'replacement-password', 'different-password', ''));
});
test('all ten languages contain every password control', () => { assert.equal(Object.keys(PasswordText.rows).length, 10); for (const row of Object.values(PasswordText.rows)) { assert.equal(row.length, PasswordText.keys.length); assert.ok(row.every(value => value.trim())); } });
test('original public responsibility is durable and paused before POST; no secret or secret hash is saved', async () => {
 const f = fixture(); await f.service.perform('', proof(), false); assert.equal(f.stops.length, 1); assert.equal(f.rows[0].state, 'changed');
 const stored = JSON.stringify(f.rows); for (const secret of Object.values(proof())) assert.ok(!stored.includes(secret));
 assert.ok(!/currentPassword|newPassword|otp|token|hash/i.test(stored)); assert.equal(f.calls[0].token, ''); assert.equal(f.calls[1].token, f.owner.token);
 assert.equal(f.calls[0].body.otp, undefined); assert.equal(f.calls[1].body.otp, '123456');
});
test('lost success resumes by anonymous lookup only using the original nonce and pair', async () => {
 const f = fixture(); f.lose = true; await assert.rejects(f.service.perform('', proof(), false)); const id = f.rows[0].id;
 await f.service.perform(id, proof(), false); assert.equal(f.calls.filter(c => c.route.endsWith('/password-change')).length, 1);
 assert.ok(f.calls.filter(c => c.route.endsWith('-results')).every(c => c.token === '' && c.body.requestId === id && !('otp' in c.body)));
});
test('unknown404 lookup cannot create; explicit retry preserves the nonce and body', async () => {
 const f = fixture(); f.lose = true; await assert.rejects(f.service.perform('', proof(), false)); f.committed = false; f.lose = false;
 const count = f.calls.length, id = f.rows[0].id; await f.service.perform(id, proof(), false); assert.equal(f.calls.length, count + 1);
 await f.service.perform(id, proof(), true); const posts = f.calls.filter(c => c.route.endsWith('/password-change')); assert.equal(posts.length, 2); assert.deepEqual(posts[0].body, posts[1].body);
});
test('unseen original never creates anonymously or borrows a successor family token', async () => {
 const f = fixture(); f.lose = true; await assert.rejects(f.service.perform('', proof(), false)); f.committed = false; f.lose = false;
 f.owner = { ...f.owner, family: uuid(), token: 'synthetic-successor-token' }; const count=f.calls.filter(c=>c.route.endsWith('/password-change')).length;
 await assert.rejects(f.service.perform(f.rows[0].id, proof(), true),/LOGIN_REQUIRED/);
 assert.equal(f.calls.filter(c=>c.route.endsWith('/password-change')).length,count); assert.equal(f.stops.length, 0);
});
test('disk failure sends zero creating requests', async () => { const f = fixture(); f.failDisk = true; await assert.rejects(f.service.perform('', proof(), false)); assert.equal(f.calls.length, 0); });
test('wrong proof or malformed result keeps an unknown responsibility paused', async () => {
 const f = fixture(); f.lose = true; await assert.rejects(f.service.perform('', proof(), false));
 f.ports.request = async () => { throw new ApiFailure(403, 'PASSWORD_CHANGE_REJECTED'); };
 await assert.rejects(f.service.perform(f.rows[0].id, proof(), false)); assert.ok(PasswordRules.blocked(f.rows[0]));
 const value = { requestId: f.rows[0].id, restoreEpoch: '2', status: 'changed', credentialGeneration: 2, securityVersion: '3', current: true, requiresNewLogin: true };
 assert.throws(() => PasswordRules.result(JSON.stringify(value), f.rows[0]));
});
test('capability withdrawal still permits old lookup but cannot start a fresh change', async () => {
 const f = fixture(); f.lose = true; await assert.rejects(f.service.perform('', proof(), false)); f.enabled = false;
 await f.service.perform(f.rows[0].id, proof(), false); assert.equal(f.rows[0].state, 'changed');
 const other = fixture(); other.enabled = false; await assert.rejects(other.service.perform('', proof(), false)); assert.equal(other.rows.length, 0);
});
test('pending is readonly until explicit same-request retry, with no new-family credentials', async () => {
 const f = fixture(); f.state = 'pending'; await f.service.perform('', proof(), false); assert.ok(PasswordRules.blocked(f.rows[0]));
 const count = f.calls.filter(c => c.route.endsWith('/password-change')).length; await f.service.perform(f.rows[0].id, proof(), false);
 assert.equal(f.calls.filter(c => c.route.endsWith('/password-change')).length, count);
 await f.service.perform(f.rows[0].id, proof(), true); assert.equal(f.calls.at(-1).token, ''); assert.equal(f.calls.at(-1).body.otp, undefined);
});
test('late identity change cannot publish or retire the successor', async () => {
 const f = fixture(), original = f.ports.request; f.ports.request = async (...args) => { const result = await original(...args); if (args[0].endsWith('/password-change')) f.active = false; return result; };
 await assert.rejects(f.service.perform('', proof(), false)); assert.equal(f.rows[0].state, 'unknown'); assert.equal(f.stops.length, 0);
});
test('metadata rejects extra secret fields and changed bindings', async () => { const f = fixture(); await f.service.perform('', proof(), false); assert.throws(() => PasswordRules.validate({ ...f.rows[0], password: 'forbidden' })); f.rows[0].profile = uuid(); await assert.rejects(f.service.perform(f.rows[0].id, proof(), false)); });

const fs=require('node:fs'), {spawn}=require('node:child_process');
const {PasswordStore}=require(path.join(work,'PasswordStore.js'));
class Bridge {
 constructor(file){this.next=0;this.waiters=new Map();this.buffer='';this.tail=Promise.resolve();this.process=spawn('python3',['-u',process.env.JOBS_IM_PASSWORD_SQLITE,file],{stdio:['pipe','pipe','pipe']});this.process.stdout.on('data',bytes=>{this.buffer+=bytes;while(this.buffer.includes('\n')){const at=this.buffer.indexOf('\n'),row=JSON.parse(this.buffer.slice(0,at));this.buffer=this.buffer.slice(at+1);const p=this.waiters.get(row.id);this.waiters.delete(row.id);row.error?p.reject(Error(row.error)):p.resolve(row.rows);}});}
 sql(sql,values=[]){const run=this.tail.then(()=>new Promise((resolve,reject)=>{const id=++this.next;this.waiters.set(id,{resolve,reject});this.process.stdin.write(JSON.stringify({id,sql,values})+'\n');}));this.tail=run.catch(()=>{});return run;}
 async executeSql(sql,values=[]){await this.sql(sql,values);}
 async querySql(sql,values=[]){const rows=await this.sql(sql,values);let at=-1;return{goToNextRow:()=>++at<rows.length,getString:i=>String(rows[at][i]),close(){}};}
 beginTransaction(){this.sql('BEGIN IMMEDIATE');}commit(){this.sql('COMMIT');}rollBack(){this.sql('ROLLBACK');}
 async close(){await this.tail;this.process.stdin.end();await new Promise(resolve=>this.process.once('exit',resolve));}
}
test('real shipped repository atomically commits pause marker and public intent, survives reopen, rejects stale CAS', async()=>{
 const f=fixture();f.lose=true;await assert.rejects(f.service.perform('',proof(),false));const row={...f.rows[0],attempts:0,state:'unsent'};
 const db=new Bridge(path.join(work,'Password-adapter.db'));let tail=Promise.resolve();const local={syncAccess(action){const job=tail.then(()=>action(db));tail=job.catch(()=>{});return job;}};
 try{
  await db.executeSql('CREATE TABLE local_state(namespace TEXT,key TEXT,value TEXT,PRIMARY KEY(namespace,key))');
  const repo=new PasswordStore(local,row.profile,()=>true);await repo.open();await repo.insert(row);
  const pending={...row,attempts:1,state:'unknown'};await repo.replace(row,pending);assert.equal((await db.sql('SELECT value FROM local_state WHERE key=?',['password-change:'+row.family]))[0][0],row.id);
  const reopened=new PasswordStore(local,row.profile,()=>true);await reopened.open();assert.deepEqual(await reopened.list(),[pending]);
  await assert.rejects(repo.replace(row,pending),/ORIGINAL_CHANGED/);
  const serialized=JSON.stringify(await db.sql('SELECT payload FROM password_change_intents'));for(const secret of Object.values(proof()))assert.ok(!serialized.includes(secret));
  await assert.rejects(repo.insert({...row,id:uuid(),state:'unsent'}),/ORIGINAL_PENDING/);
  const rejected={...pending,state:'rejected'};await reopened.replace(pending,rejected);assert.equal((await db.sql('SELECT value FROM local_state WHERE key=?',['password-change:'+row.family]))[0][0],'');
  await repo.insert({...row,id:uuid(),state:'unsent'});
 }finally{await db.close();}
});
test('fresh change validates current family before responsibility and uses refreshed same-family AT',async()=>{
 const f=fixture();let prepared=false;f.ports.prepare=async()=>{assert.equal(f.rows.length,0);prepared=true;f.owner={...f.owner,token:'synthetic-refreshed-original-family'};};
 await f.service.perform('',proof(),false);assert.ok(prepared);assert.equal(f.calls.find(c=>c.route.endsWith('/password-change')).token,'synthetic-refreshed-original-family');
});
test('capability withdrawal and restore changes permit only lookup, never pending or unseen mutation',async()=>{
 for(const restored of [false,true]) for(const state of ['pending','unseen']) {
  const f=fixture();f.lose=true;await assert.rejects(f.service.perform('',proof(),false));f.lose=false;f.state='pending';f.committed=state==='pending';
  if(restored)f.ports.bootstrap=async()=>({deploymentId:f.ports.deployment,restoreEpoch:'3',features:{own_password_change:true}});else f.enabled=false;
  const count=f.calls.filter(c=>c.route.endsWith('/password-change')).length;
  await f.service.perform(f.rows[0].id,proof(),false);
  await assert.rejects(f.service.perform(f.rows[0].id,proof(),true),/UNAVAILABLE/);
  assert.equal(f.calls.filter(c=>c.route.endsWith('/password-change')).length,count);
  assert.ok(f.calls.slice(-2).every(c=>c.route.endsWith('-results')&&c.token===''));
 }
});
