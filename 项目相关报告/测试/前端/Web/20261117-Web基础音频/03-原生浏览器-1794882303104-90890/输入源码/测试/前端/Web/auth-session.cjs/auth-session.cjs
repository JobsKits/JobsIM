// Created by Jobs on 2026年11月15日，星期日.
// Uses a real browser and simulated BFF responses; this is not HTTPS integration evidence.
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const { randomUUID } = require('node:crypto');
const base = process.env.JOBSIM_WEB_URL || 'http://127.0.0.1:5188';
const out = process.env.JOBSIM_AUTH_REPORT || path.resolve(__dirname, '../../../../项目相关报告/测试/前端/Web/执行记录/auth-session-' + Date.now() + '-' + process.pid);

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' });
  const context = await browser.newContext();
  const deployment = randomUUID(), alice = randomUUID(), bob = randomUUID();
  let family = randomUUID(), device = randomUUID(), user = alice, username = 'alice';
  let generation = 0, expired = false, dropRefresh = false, dropLogin = false;
  let refreshCalls = 0, initialCalls = 0, learnCalls = 0, held = null, release = null;
  let chainSupported = true;
  let refreshSupported = true;
  const seenWrites = [], checks = [];
  const metadata = () => ({ familyId: family, refreshGeneration: String(generation), accessExpiresAt: new Date(Date.now() + 900000).toISOString(), idleExpiresAt: new Date(Date.now() + 2592000000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 7776000000).toISOString(), sourceContextId: 'personal', restoreEpoch: '1', deviceId: device, user: { id: user, username, displayName: username } });
  const error = (route, status, code) => route.fulfill({ status, json: { error: { code, message: code } } });
  await context.route('**/__auth-tests', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><html><title>JobsIM auth protocol harness</title></html>' }));
  await context.route('**/api/**', async route => {
    const req = route.request(), pathname = new URL(req.url()).pathname;
    if (pathname === '/api/bootstrap') return route.fulfill({ json: { deploymentId: deployment, features: { auth_refresh: refreshSupported, auth_retirement_chain: chainSupported } } });
    if (['/api/auth/login', '/api/auth/register', '/api/auth/reauthenticate'].includes(pathname)) {
      initialCalls++;
      if (dropLogin) return route.abort('failed');
      const body = req.postDataJSON();
      username = body.username; user = username === 'bob' ? bob : alice;
      family = randomUUID(); device = randomUUID(); generation = 0; expired = false;
      return route.fulfill({ json: metadata() });
    }
    const header = req.headers()['x-jobsim-auth-family'];
    const learning = req.headers()['x-jobsim-context-learn'] === '1' && ['/api/auth/context', '/api/auth/refresh-context'].includes(pathname);
    if (header !== family && !learning) return error(route, 409, 'AUTH_CONTEXT_CHANGED');
    if (learning) learnCalls++;
    if (pathname === '/api/auth/refresh-context') {
      const { user: ignoredUser, deviceId: ignoredDevice, ...publicMetadata } = metadata();
      return route.fulfill({ json: { ...publicMetadata, csrfToken: 'test-csrf' } });
    }
    if (pathname === '/api/auth/refresh') {
      refreshCalls++;
      const body = req.postDataJSON();
      assert.equal(body.familyId, family);
      assert.equal(body.refreshGeneration, String(generation));
      assert.equal(req.headers()['x-csrf-token'], 'test-csrf');
      assert.equal('refreshToken' in body, false);
      generation++; expired = false;
      if (dropRefresh) return route.abort('failed');
      await new Promise(resolve => setTimeout(resolve, 50));
      return route.fulfill({ json: metadata() });
    }
    if (expired) return error(route, 401, 'ACCESS_EXPIRED');
    if (pathname === '/api/auth/csrf') return route.fulfill({ json: { csrfToken: 'test-csrf' } });
    if (pathname === '/api/auth/context') return route.fulfill({ json: metadata() });
    if (pathname === '/api/auth/confirm') return route.fulfill({ json: {} });
    if (pathname === '/api/held') {
      held?.();
      const result = metadata();
      await new Promise(resolve => { release = resolve; });
      return route.fulfill({ json: result });
    }
    if (pathname === '/api/media') return route.fulfill({ body: 'private-bytes', contentType: 'application/octet-stream' });
    if (req.method() === 'POST') seenWrites.push(req.postDataJSON());
    return route.fulfill({ json: { ok: true, familyId: family } });
  });
  const page = await context.newPage();
  const open = async target => {
    await target.goto(base + '/__auth-tests');
    await target.evaluate(async () => {
      // Vite appends HMR timestamps to dependencies; import the same module instance.
      const source = await (await fetch('/Src/api.ts')).text();
      const authURL = source.match(/from\s+["']([^"']*AuthSession\.ts[^"']*)["']/)?.[1];
      if (!authURL) throw Error('AuthSession dependency was not found');
      window.auth = await import(authURL);
      window.transport = await import('/Src/api.ts');
      await window.transport.api('/bootstrap');
    });
  };
  try {
    await open(page);
    await page.evaluate(async () => {
      const intent = window.auth.prepareInitial('login', 'alice');
      await window.transport.api('/auth/login', window.transport.json('POST', { username: 'alice', password: 'fixture-only', requestId: intent.requestId }));
      await window.transport.api('/auth/confirm', window.transport.json('POST', {}));
      await window.transport.api('/auth/context');
      window.auth.finishActivatedInitial(window.auth.authSnapshot().auth.familyId);
    });
    expired = true;
    await page.evaluate(() => Promise.all(Array.from({ length: 12 }, (_, i) => window.transport.api('/write', window.transport.json('POST', { clientMsgId: 'original-' + i, text: 'unchanged-' + i })))));
    assert.equal(refreshCalls, 1);
    assert.deepEqual(seenWrites.map(v => v.clientMsgId).sort(), Array.from({ length: 12 }, (_, i) => 'original-' + i).sort());
    checks.push('12 concurrent writes: one refresh, original business payloads');

    const sibling = await context.newPage();
    await open(sibling);
    expired = true;
    await Promise.all([page, sibling].map(target => target.evaluate(() => Promise.all(Array.from({ length: 6 }, () => window.transport.api('/read'))))));
    assert.equal(refreshCalls, 2);
    checks.push('two browser tabs: Web Locks prevent a second refresh consume');
    await sibling.close();

    expired = true; dropRefresh = true;
    const failed = await page.evaluate(async () => { try { await window.transport.api('/write', window.transport.json('POST', { clientMsgId: 'unknown-original' })); } catch (e) { return e.code; } });
    assert.equal(failed, 'AUTH_REFRESH_UNKNOWN');
    const unknownCount = refreshCalls;
    await open(page);
    const stopped = await page.evaluate(async () => { try { await window.transport.api('/read'); } catch (e) { return e.code; } });
    assert.equal(stopped, 'AUTH_REFRESH_UNKNOWN');
    assert.equal(refreshCalls, unknownCount);
    assert.equal(seenWrites.some(v => v.clientMsgId === 'unknown-original'), false);
    checks.push('lost committed refresh response stays blocked after reload without replay');
    dropRefresh = false;

    const oldFamily = family;
    const reauth = await page.evaluate(async () => {
      const intent = window.auth.prepareInitial('login', 'alice');
      await window.transport.api('/auth/' + intent.kind, window.transport.json('POST', { ...intent, password: 'fixture-only' }));
      await window.transport.api('/auth/confirm', window.transport.json('POST', {}));
      await window.transport.api('/auth/context');
      window.auth.finishActivatedInitial(window.auth.authSnapshot().auth.familyId);
      return intent;
    });
    assert.equal(reauth.kind, 'reauthenticate');
    assert.equal(reauth.previousFamilyId, oldFamily);
    checks.push('fresh proof links the unknown old family and clears only its recovery marker');

    dropLogin = true;
    const intentResult = await page.evaluate(async () => {
      const first = window.auth.prepareInitial('register', 'new-person');
      try { await window.transport.api('/auth/register', window.transport.json('POST', { username: 'new-person', password: 'fixture-only', requestId: first.requestId })); } catch {}
      const retry = window.auth.prepareInitial('register', 'new-person');
      window.auth.expireInitial(first.requestId);
      const successor = window.auth.prepareInitial('register', 'new-person');
      const retrySuccessor = window.auth.prepareInitial('register', 'new-person');
      return { first, retry, successor, retrySuccessor };
    });
    assert.equal(intentResult.retry.requestId, intentResult.first.requestId);
    assert.equal(intentResult.retry.kind, 'register');
    assert.equal(intentResult.successor.previousRequestId, intentResult.first.requestId);
    assert.deepEqual(intentResult.successor.previousRequestIds, [intentResult.first.requestId]);
    assert.equal(intentResult.retrySuccessor.requestId, intentResult.successor.requestId);
    checks.push('unknown initial and unknown successor retain their original nonces');
    dropLogin = false;

    const started = new Promise(resolve => { held = resolve; });
    const late = page.evaluate(async () => { try { await window.transport.api('/held'); return 'accepted'; } catch (e) { return e.code; } });
    await started;
    await page.evaluate(async () => {
      const intent = window.auth.prepareInitial('login', 'bob');
      await window.transport.api('/auth/' + intent.kind, window.transport.json('POST', { ...intent, password: 'fixture-only' }));
      await window.transport.api('/auth/confirm', window.transport.json('POST', {}));
    });
    release();
    assert.equal(await late, 'AUTH_REQUEST_ABORTED');
    await page.evaluate(() => window.transport.api('/read'));
    checks.push('late Alice response cannot replace or revoke the newly verified Bob context');

    const media = await page.evaluate(async () => (await window.transport.apiBytes('/media')).text());
    assert.equal(media, 'private-bytes');
    const storage = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, tab: { ...sessionStorage } }));
    assert.equal(storage.includes('fixture-only'), false);
    assert.equal(storage.includes('refreshToken'), false);
    assert.equal(storage.includes('"token"'), false);
    checks.push('private bytes carry the family header; storage contains no credentials');

    expired = true;
    const beforeFault = refreshCalls;
    await page.evaluate(() => {
      const original = Storage.prototype.setItem;
      Storage.prototype.setItem = function(key, value) {
        if (key.startsWith('jobsim.auth.recovery|')) throw new DOMException('fixture quota', 'QuotaExceededError');
        return original.call(this, key, value);
      };
    });
    const failure = await page.evaluate(async () => { try { await window.transport.api('/read'); } catch (e) { return e.message; } });
    assert.equal(failure, 'AUTH_STORAGE_UNAVAILABLE');
    assert.equal(refreshCalls, beforeFault);
    checks.push('failed pending-state persistence prevents the refresh POST');

    await page.evaluate(() => { localStorage.clear(); sessionStorage.clear(); });
    const cold = await context.newPage();
    await open(cold);
    await cold.evaluate(() => window.transport.api('/auth/context'));
    assert.equal(refreshCalls, beforeFault + 1);
    assert.equal(learnCalls > 0, true);
    checks.push('cold browser learns only the public expired context before one refresh');
    const isolated = await cold.evaluate(() => {
      const first = window.auth.prepareInitial('register', 'isolated-person');
      window.auth.configureAuthProtocol(true, crypto.randomUUID());
      const next = window.auth.prepareInitial('register', 'isolated-person');
      return { first, next };
    });
    assert.notEqual(isolated.first.requestId, isolated.next.requestId);
    assert.equal(isolated.next.previousRequestId, undefined);
    assert.equal(isolated.next.previousFamilyId, undefined);
    checks.push('initial authentication intents do not cross deployment identities');
    await cold.evaluate(async () => {
      const store = await import('/Src/storage.ts');
      const { emptySpace } = await import('/Src/types.ts');
      const scope = location.origin + '|fixture-deployment|fixture-user|personal';
      const legacy = scope + '|legacy-device', old = scope + '|1|old-device', current = scope + '|1|new-device';
      const space = emptySpace();
      const message = { id: 'pending', clientMsgId: 'original-intent', conversationId: 'fixture-conversation', type: 'text', text: 'preserved draft', version: '1', pending: true, restoreEpoch: '1', sendAdmission: 'original-permit' };
      await store.queueMessage(legacy, space, message);
      await store.queueMessage(old, space, message);
      await store.isolateAuthNamespace(scope, current);
      await store.queueMessage(old, space, { ...message, clientMsgId: 'late-original' });
      const firstLate = scope + '|1|first-late-device';
      const firstReplace = scope + '|1|first-replaced-device';
      await store.queueMessage(firstLate, space, message);
      await store.saveSpace(firstReplace, { ...space, outbox: [message] });
      for (const key of [legacy, old, firstLate, firstReplace]) {
        const snapshot = await store.loadSpace(key);
        if (!snapshot.outbox.length || snapshot.outbox.some(row => !row.frozen || row.text !== message.text || row.sendAdmission !== message.sendAdmission)) throw Error('Old queue was replaced or left live');
        if (await store.prepareOutboxAttempt(key, message.clientMsgId)) throw Error('Frozen auth namespace can still send');
      }
      if ((await store.loadSpace(current)).outbox.length) throw Error('Reauthentication moved an old queue');
    });
    checks.push('IndexedDB transaction freezes legacy/old device queues and late enqueue without moving drafts');
    const proofGuard = await cold.evaluate(() => {
      window.auth.configureAuthProtocol(true, crypto.randomUUID(), true);
      const original = window.auth.prepareInitial('login', 'proof-person');
      window.auth.retainInitialProof(original.requestId, 'original-only-in-memory');
      let changed = '';
      try { window.auth.retainInitialProof(original.requestId, 'changed-only-in-memory'); } catch (error) { changed = error.message; }
      const successor = window.auth.prepareInitial('login', 'proof-person', true);
      return { changed, original, successor, stored: JSON.stringify({ ...localStorage }) };
    });
    assert.equal(proofGuard.changed, 'AUTH_ORIGINAL_PROOF_CHANGED');
    assert.notEqual(proofGuard.successor.requestId, proofGuard.original.requestId);
    assert.deepEqual(proofGuard.successor.previousRequestIds, [proofGuard.original.requestId]);
    assert.equal(proofGuard.stored.includes('original-only-in-memory'), false);
    assert.equal(proofGuard.stored.includes('changed-only-in-memory'), false);
    checks.push('changed proof cannot reuse an old intent; explicit fresh proof carries its retirement chain without storing secrets');
    const peer = await context.newPage();
    await open(peer);
    const concurrent = await Promise.all([cold, peer].map((target, i) => target.evaluate(async ({ deploymentId, index }) => {
      window.auth.configureAuthProtocol(true, deploymentId, true);
      return window.transport.withAuthLifecycle(async () => {
        const intent = window.auth.prepareInitial('login', 'concurrent-' + index);
        await new Promise(resolve => setTimeout(resolve, 60));
        window.auth.receiveInitial(intent.requestId, crypto.randomUUID());
        return intent.requestId;
      });
    }, { deploymentId: deployment, index: i })));
    const retained = await cold.evaluate(() => window.auth.initialAuthIntents().map(item => item.requestId));
    assert.equal(concurrent.every(id => retained.includes(id)), true);
    checks.push('full lifecycle lock retains both concurrent tab intents through preparation and receipt updates');
    const guarded = await cold.evaluate(() => {
      const first = window.auth.prepareInitial('login', 'chain-gate');
      window.auth.prepareInitial('login', 'chain-gate', true);
      const third = window.auth.prepareInitial('login', 'chain-gate', true);
      const body = JSON.stringify({ ...third, password: 'fixture-only' });
      window.auth.configureAuthProtocol(true, third.deploymentId, false);
      let code = '';
      try { window.auth.prepareInitial('login', 'chain-gate'); } catch (error) { code = error.message; }
      return { code, body, first };
    });
    assert.equal(guarded.code, 'AUTH_RECOVERY_REQUIRED');
    chainSupported = false;
    const beforeGate = initialCalls;
    const gate = await cold.evaluate(async body => {
      try { await window.transport.api('/auth/reauthenticate', { method: 'POST', body }); } catch (error) { return error.code; }
    }, guarded.body);
    assert.equal(gate, 'AUTH_RECOVERY_REQUIRED');
    assert.equal(initialCalls, beforeGate);
    checks.push('retained multi-level body is refused before POST when live chain capability disappears');
    const heldLifecycle = peer.evaluate(() => window.transport.withAuthLifecycle(async () => {
      window.lifecycleHeld = true;
      await new Promise(resolve => { window.releaseLifecycle = resolve; });
    }));
    await peer.waitForFunction(() => window.lifecycleHeld === true);
    const cancelled = cold.evaluate(async () => {
      try {
        await window.transport.withAuthLifecycle(async () => { window.cancelledActionRan = true; });
        return 'ran';
      } catch (error) { return error.name; }
    });
    await cold.waitForTimeout(30);
    await cold.evaluate(() => window.transport.resetRequestContext());
    await peer.evaluate(() => window.releaseLifecycle());
    await heldLifecycle;
    assert.equal(await cancelled, 'AbortError');
    assert.equal(await cold.evaluate(() => window.cancelledActionRan === true), false);
    checks.push('context reset cancels queued lifecycle work before it can submit credentials');
    refreshSupported = false;
    const beforeDowngrade = initialCalls;
    const downgraded = await cold.evaluate(async () => {
      await window.transport.api('/bootstrap');
      try { await window.transport.api('/auth/login', window.transport.json('POST', { username: 'alice', password: 'fixture-only', requestId: crypto.randomUUID() })); } catch (error) { return error.code; }
    });
    assert.equal(downgraded, 'AUTH_RECOVERY_REQUIRED');
    assert.equal(initialCalls, beforeDowngrade);
    checks.push('missing refresh capability cannot bypass retained responsibility through a new legacy login ID');
    await peer.close();
    await cold.close();
    fs.mkdirSync(out, { recursive: true });
    fs.writeFileSync(path.join(out, '浏览器协议结果.json'), JSON.stringify({ simulatedBFF: true, realBackend: false, checks, refreshCalls, initialCalls, learnCalls, date: new Date().toISOString() }, null, 2));
    console.log('PASS browser auth protocol:', checks.length, 'groups; simulated BFF, not real HTTPS');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
