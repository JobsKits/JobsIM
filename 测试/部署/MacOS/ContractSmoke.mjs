import assert from 'node:assert/strict';
import { randomBytes, randomUUID } from 'node:crypto';
const base=process.env.JOBS_IM_TEST_URL;
assert.ok(base?.startsWith('https://'),'Explicit isolated HTTPS test URL required');
const password=randomBytes(24).toString('base64url');
const suffix=Date.now().toString();
async function api(path,method='GET',body,token='') {
  const response=await fetch(base+'/api'+path,{method,redirect:'error',headers:{'Content-Type':'application/json',...(token?{Authorization:'Bearer '+token}:{})},body:body===undefined?undefined:JSON.stringify(body),signal:AbortSignal.timeout(8000)});
  const json=await response.json();
  assert.ok(response.ok,`${method} ${path}: HTTP ${response.status} ${json.error?.code??''}`);
  return json;
}
const bootstrap=await api('/bootstrap');
assert.equal(bootstrap.protocolVersion,'1');
assert.match(bootstrap.restoreEpoch,/^[1-9][0-9]*$/);
assert.equal(bootstrap.features?.send_admission,true,'Current sending admission protocol required');
const a=await api('/auth/register','POST',{username:'mac_a_'+suffix,password,requestId:randomUUID(),deviceName:'Mac deployment smoke'});
const b=await api('/auth/register','POST',{username:'mac_b_'+suffix,password,requestId:randomUUID(),deviceName:'Mac deployment smoke'});
try {
  const request=await api('/contacts/requests','POST',{userId:b.user.id,requestId:randomUUID()},a.token);
  await api('/contacts/requests/'+request.id+'/accept','POST',{},b.token);
  const conversation=await api('/conversations','POST',{kind:'direct',title:'Deployment smoke',memberIds:[b.user.id]},a.token);
  assert.ok(conversation.sendAdmission,'Original conversation admission required');
  const payload={clientMsgId:randomUUID(),type:'text',text:'Mac deployment isolated fixture',restoreEpoch:bootstrap.restoreEpoch,sendAdmission:conversation.sendAdmission};
  const sent=await api('/conversations/'+conversation.id+'/messages','POST',payload,a.token);
  const retried=await api('/conversations/'+conversation.id+'/messages','POST',payload,a.token);
  assert.equal(sent.id,retried.id);
  const received=await api('/conversations/'+conversation.id+'/messages?after='+bootstrap.restoreEpoch+':0&limit=100','GET',undefined,b.token);
  assert.ok(received.items.some(message=>message.id===sent.id));
  console.log('PASS: strict TLS, two registered accounts, contact acceptance, direct text, original-ID retry and recipient history');
  console.log('Deployment '+bootstrap.deploymentId+'; restoreEpoch='+bootstrap.restoreEpoch);
} finally {
  await api('/auth/logout','POST',{},a.token);
  await api('/auth/logout','POST',{},b.token);
}
