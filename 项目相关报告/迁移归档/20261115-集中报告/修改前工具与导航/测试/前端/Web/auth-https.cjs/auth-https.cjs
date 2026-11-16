// Created by Jobs on 2026年11月15日，星期日.
// Explicit isolated-realm test: SQL changes only the test-created family's expiry.
const fs = require('node:fs'), path = require('node:path'), https = require('node:https');
const crypto = require('node:crypto'), assert = require('node:assert/strict'), cp = require('node:child_process');
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || 'playwright');
const seedPath = process.env.JOBSIM_AUTH_TEST_SEED;
if (!seedPath) throw Error('Provide an explicit private isolated-realm seed');
const seed = JSON.parse(fs.readFileSync(seedPath, 'utf8'));
if (!seed.refreshTestRealm || !seed.persistentData) throw Error('An isolated persistent refresh realm is required');
const server = seed.serverURL, agent = new https.Agent({ ca: fs.readFileSync(seed.caPath) });
const cert = path.join(path.dirname(seed.caPath), 'server.pem');
const pin = crypto.createHash('sha256').update(new crypto.X509Certificate(fs.readFileSync(cert)).publicKey.export({ type: 'spki', format: 'der' })).digest('base64');
const out = process.env.JOBSIM_AUTH_REPORT || path.resolve(__dirname, '../报告/20261115-登录续期HTTPS');
function sql(query) {
  const url = new URL(seed.businessURL);
  return cp.execFileSync(path.join(seed.pgBin, 'psql'), ['-X', '-A', '-t', '-v', 'ON_ERROR_STOP=1', '-c', query], { encoding: 'utf8', env: { ...process.env, PGHOST: url.hostname, PGPORT: url.port, PGDATABASE: url.pathname.slice(1), PGUSER: decodeURIComponent(url.username), PGPASSWORD: decodeURIComponent(url.password), PGPASSFILE: seed.pgPassFile || '' }, stdio: ['ignore', 'pipe', 'pipe'] }).trim();
}
function native(route, method = 'GET', body, token) {
  return new Promise((resolve, reject) => {
    const request = https.request(server + '/api' + route, { method, agent, headers: { 'Content-Type': 'application/json', ...(token ? { Authorization: 'Bearer ' + token } : {}) } }, response => {
      const chunks = [];
      response.on('data', chunk => chunks.push(chunk));
      response.on('end', () => {
        const data = JSON.parse(Buffer.concat(chunks).toString() || '{}');
        response.statusCode >= 400 ? reject(Error(response.statusCode + '/' + data.error?.code)) : resolve(data);
      });
    });
    request.on('error', reject);
    request.end(body === undefined ? undefined : JSON.stringify(body));
  });
}
function verifiedForward(request, capture = false) {
  return request.allHeaders().then(headers => new Promise((resolve, reject) => {
    const outgoing = https.request(request.url(), { method: request.method(), headers, agent }, response => {
      const chunks = [];
      if (capture) response.on('data', chunk => chunks.push(chunk));
      else response.resume();
      response.on('end', () => resolve(capture ? { status: response.statusCode, metadata: JSON.parse(Buffer.concat(chunks).toString()) } : response.statusCode));
    });
    outgoing.on('error', reject);
    outgoing.end(request.postDataBuffer() || undefined);
  }));
}
function familySQL(family) {
  assert.match(family, /^[a-f0-9-]{36}$/i);
  return "family_id='" + family + "'";
}
(async () => {
  const boot = await native('/bootstrap');
  assert.equal(boot.features.auth_refresh, true);
  const stamp = Date.now().toString(36), password = 'JobsIM refresh test ' + crypto.randomBytes(12).toString('hex');
  const a = await native('/auth/register', 'POST', { username: 'web_auth_a_' + stamp, password, requestId: crypto.randomUUID(), deviceName: 'Web auth fixture A' });
  const b = await native('/auth/register', 'POST', { username: 'web_auth_b_' + stamp, password, requestId: crypto.randomUUID(), deviceName: 'Web auth fixture B' });
  const invitation = await native('/contacts/requests', 'POST', { userId: b.user.id, requestId: crypto.randomUUID() }, a.token);
  await native('/contacts/requests/' + invitation.id + '/accept', 'POST', {}, b.token);
  const title = 'Auth HTTPS ' + stamp;
  const conversation = await native('/conversations', 'POST', { kind: 'group', title, memberIds: [b.user.id], requestId: crypto.randomUUID() }, a.token);
  const browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', args: ['--ignore-certificate-errors-spki-list=' + pin] });
  const context = await browser.newContext({ viewport: { width: 1440, height: 960 } });
  const page = await context.newPage(), checks = [], errors = [];
  let refreshCalls = 0, dropRefresh = false, pendingBody = null, pendingAttempts = 0;
  let initialPosts = 0, dropInitial = false, lostInitialFamily = '';
  const pendingText = 'Retain original auth draft ' + stamp;
  let submitted;
  const pendingSubmitted = new Promise(resolve => { submitted = resolve; });
  const authState = () => page.evaluate(() => JSON.parse(localStorage.getItem('jobsim.auth.active') || 'null'));
  const login = async () => {
    await page.locator('input[name=username]').fill(a.user.username);
    await page.locator('input[name=password]').fill(password);
    await page.locator('.modal form').getByRole('button', { name: '登录', exact: true }).click();
    await page.locator('.modal-overlay').waitFor({ state: 'hidden' });
    await page.locator('.conversation-row').filter({ hasText: title }).click();
  };
  await context.route('**/api/auth/refresh', async route => {
    refreshCalls++;
    if (!dropRefresh) return route.continue();
    dropRefresh = false;
    assert.equal(await verifiedForward(route.request()), 200);
    await route.abort('failed');
  });
  await context.route(/\/api\/auth\/(login|reauthenticate)$/, async route => {
    initialPosts++;
    if (!dropInitial) return route.continue();
    dropInitial = false;
    const response = await verifiedForward(route.request(), true);
    assert.equal(response.status, 200);
    lostInitialFamily = response.metadata.familyId;
    await route.abort('failed');
  });
  await context.route('**/api/conversations/' + conversation.id + '/messages', route => {
    if (route.request().method() === 'POST' && route.request().postDataJSON()?.text === pendingText) {
      pendingBody = route.request().postDataJSON(); pendingAttempts++;
      submitted();
      return route.abort('failed');
    }
    return route.continue();
  });
  page.on('pageerror', error => errors.push(error.message));
  page.on('dialog', dialog => dialog.accept());
  try {
    await page.goto(server);
    await page.getByRole('button', { name: '登录', exact: true }).click();
    await login();
    const first = await authState();
    const cookies = await context.cookies(server);
    const cookie = cookies.find(item => item.name === '__Host-jobsim_session');
    assert.ok(cookie?.httpOnly && cookie.secure && cookie.path === '/');
    assert.equal(await page.evaluate(() => document.cookie.includes('__Host-jobsim_session')), false);
    const storage = await page.evaluate(() => JSON.stringify({ local: { ...localStorage }, tab: { ...sessionStorage } }));
    assert.equal(storage.includes(password), false);
    assert.equal(storage.includes('refreshToken'), false);
    checks.push('production UI login: real HttpOnly Secure host cookie, no JS credentials');

    const sibling = await context.newPage();
    await sibling.goto(server);
    await sibling.locator('.conversation-row').filter({ hasText: title }).waitFor();
    sql('UPDATE auth.sessions SET access_expires_at=now()-interval \'1 second\' WHERE ' + familySQL(first.familyId));
    const advanced = String(BigInt(first.refreshGeneration) + 1n);
    await page.waitForFunction(expected => JSON.parse(localStorage.getItem('jobsim.auth.active') || 'null')?.refreshGeneration === expected, advanced);
    await page.waitForTimeout(500);
    assert.equal(refreshCalls, 1);
    assert.equal(sql('SELECT refresh_generation::text FROM auth.sessions WHERE ' + familySQL(first.familyId)), advanced, 'actual family generation advances exactly once');
    assert.equal((await authState()).deviceId, first.deviceId);
    await sibling.close();
    checks.push('two real UI tabs consume one refresh and retain the same device/family');

    await page.locator('textarea').fill(pendingText);
    await page.getByRole('button', { name: '发送', exact: true }).click();
    let pendingDeadline;
    try {
      await Promise.race([pendingSubmitted, new Promise((_, reject) => { pendingDeadline = setTimeout(() => reject(Error('Original pending POST was not observed')), 30000); })]);
    } finally {
      clearTimeout(pendingDeadline);
    }
    assert.ok(pendingBody?.clientMsgId);
    const oldNamespace = server + '|' + boot.deploymentId + '|' + a.user.id + '|personal|' + boot.restoreEpoch + '|' + first.deviceId;
    dropRefresh = true;
    sql('UPDATE auth.sessions SET access_expires_at=now()-interval \'1 second\' WHERE ' + familySQL(first.familyId));
    await page.locator('.modal form input[name=username]').waitFor();
    assert.equal(refreshCalls, 2);
    await page.reload();
    await page.locator('.modal form input[name=username]').waitFor();
    assert.equal(refreshCalls, 2);
    checks.push('committed refresh response loss and reload stop automatic refresh replay');

    await login();
    const next = await authState();
    assert.notEqual(next.familyId, first.familyId);
    assert.notEqual(next.deviceId, first.deviceId);
    assert.equal(sql('SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE ' + familySQL(first.familyId)), 'true');
    const oldQueue = await page.evaluate(key => new Promise((resolve, reject) => {
      const open = indexedDB.open('JobsIM', 1);
      open.onsuccess = () => {
        const read = open.result.transaction('spaces').objectStore('spaces').get('outbox|' + key);
        read.onsuccess = () => { resolve(read.result); open.result.close(); };
        read.onerror = () => reject(read.error);
      };
      open.onerror = () => reject(open.error);
    }), oldNamespace);
    const original = oldQueue.find(item => item.clientMsgId === pendingBody.clientMsgId);
    assert.ok(original?.frozen);
    assert.equal(original.text, pendingText);
    assert.equal(original.sendAdmission, pendingBody.sendAdmission);
    assert.equal(pendingAttempts, 1);
    await native('/me', 'GET', undefined, a.token);
    checks.push('verified reauthentication retires only the old Web family; original queue stays frozen in its old namespace');

    const message = 'New verified session ' + stamp;
    await page.locator('textarea').fill(message);
    const accepted = page.waitForResponse(response => response.request().method() === 'POST' && response.url() === server + '/api/conversations/' + conversation.id + '/messages' && response.request().postDataJSON()?.text === message);
    await page.getByRole('button', { name: '发送', exact: true }).click();
    assert.equal((await accepted).status(), 200);
    await page.locator('.message-content').getByText(message, { exact: true }).waitFor();
    assert.equal(errors.length, 0, errors.join('\n'));
    checks.push('new session sends normally through production UI without moving the old draft');
    const renewedGeneration = String(BigInt(next.refreshGeneration) + 1n);
    sql('UPDATE auth.sessions SET access_expires_at=now()-interval \'1 second\' WHERE ' + familySQL(next.familyId));
    await page.evaluate(() => { localStorage.removeItem('jobsim.auth.active'); sessionStorage.removeItem('jobsim.auth.context'); });
    await page.reload();
    await page.locator('.conversation-row').filter({ hasText: title }).waitFor();
    await page.waitForFunction(expected => JSON.parse(localStorage.getItem('jobsim.auth.active') || 'null')?.refreshGeneration === expected, renewedGeneration);
    assert.equal((await authState()).familyId, next.familyId);
    assert.equal(refreshCalls, 3);
    assert.equal(pendingAttempts, 1);
    checks.push('expired opaque Cookie with no JS metadata learns only public context and renews the same family once');
    fs.mkdirSync(out, { recursive: true });
    await page.getByRole('button', { name: '设置', exact: true }).click();
    await page.getByRole('button', { name: '退出登录', exact: true }).click();
    await page.locator('.settings-content').getByRole('button', { name: '登录', exact: true }).waitFor();
    const holder = await context.newPage();
    await holder.route(server + '/__auth_lifecycle_hold', route => route.fulfill({ contentType: 'text/html', body: '<!doctype html><title>Lock fixture</title>' }));
    await holder.goto(server + '/__auth_lifecycle_hold');
    const held = holder.evaluate(() => navigator.locks.request('jobsim-auth-lifecycle', async () => {
      window.holding = true;
      await new Promise(resolve => { window.release = resolve; });
    }));
    await holder.waitForFunction(() => window.holding === true);
    await page.locator('.settings-content').getByRole('button', { name: '登录', exact: true }).click();
    await page.locator('input[name=username]').fill(a.user.username);
    await page.locator('input[name=password]').fill(password);
    const beforeCancelled = initialPosts;
    await page.locator('.modal form').getByRole('button', { name: '登录', exact: true }).click();
    await page.waitForTimeout(60);
    await page.locator('.modal').getByRole('button', { name: '关闭', exact: true }).click();
    await holder.evaluate(() => window.release());
    await held;
    await page.waitForTimeout(120);
    assert.equal(initialPosts, beforeCancelled);
    await holder.close();
    checks.push('closing a queued production auth form prevents a later credentials POST after the other tab releases its lock');

    dropInitial = true;
    await page.locator('.settings-content').getByRole('button', { name: '登录', exact: true }).click();
    await page.locator('input[name=username]').fill(a.user.username);
    await page.locator('input[name=password]').fill(password);
    await page.locator('.modal form').getByRole('button', { name: '登录', exact: true }).click();
    await page.getByText('登录结果待确认。请再次提交相同账号，核对原登录请求。', { exact: true }).waitFor();
    const beforeChanged = initialPosts;
    await page.locator('input[name=password]').fill(password + ' changed');
    await page.locator('.modal form').getByRole('button', { name: '登录', exact: true }).click();
    await page.getByText('输入与原请求不同。请使用重新验证身份，安全关闭原请求。', { exact: true }).waitFor();
    assert.equal(initialPosts, beforeChanged);
    await page.screenshot({ path: path.join(out, 'Web-原证明保护与关联验证.png'), fullPage: true });
    await page.locator('input[name=password]').fill(password);
    await page.getByRole('button', { name: '重新验证身份', exact: true }).click();
    await page.locator('.modal-overlay').waitFor({ state: 'hidden' });
    await page.locator('.conversation-row').filter({ hasText: title }).click();
    assert.equal(sql('SELECT (revoked_at IS NOT NULL)::text FROM auth.sessions WHERE ' + familySQL(lostInitialFamily)), 'true');
    assert.equal(pendingAttempts, 1);
    checks.push('lost real initial response keeps its nonce; changed proof is rejected before POST and explicit new proof retires the original family');
    await page.screenshot({ path: path.join(out, 'Web-真实续期与重新认证.png'), fullPage: true });
    fs.writeFileSync(path.join(out, '真实联调结果.json'), JSON.stringify({ realBackend: true, tlsVerifiedByCAAndPinnedBrowserCertificate: true, productionDist: true, checks, refreshCalls, pendingAttempts, errors, date: new Date().toISOString() }, null, 2));
    console.log('PASS real Web auth HTTPS:', checks.length, 'groups; private fixture values not logged');
  } catch (error) {
    fs.mkdirSync(out, { recursive: true });
    await page.screenshot({ path: path.join(out, '未验收-失败诊断.png'), fullPage: true }).catch(() => {});
    const diagnostic = await page.evaluate(() => ({
      notices: [...document.querySelectorAll('.notice')].map(item => item.textContent),
      initial: JSON.parse(localStorage.getItem('jobsim.auth.initial-intents') || '[]').map(item => ({ kind: item.kind, expired: item.expired === true, received: !!item.receivedFamilyId, confirmed: item.confirmed === true })),
    })).catch(() => ({}));
    console.error(JSON.stringify({ initialPosts, refreshCalls, diagnostic }));
    throw error;
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error.message); process.exitCode = 1; });
