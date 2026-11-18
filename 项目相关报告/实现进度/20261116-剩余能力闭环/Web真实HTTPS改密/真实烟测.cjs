// Created by Jobs on 2026年11月17日，星期二.
// Own Canonical realm only. Passwords, Cookie, CSRF and tokens stay in RAM.
const fs = require('node:fs'), path = require('node:path'), crypto = require('node:crypto'), https = require('node:https');
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE);
const root = process.cwd(), origin = 'https://127.0.0.1:55443';
const state = path.join(process.env.HOME, 'Library/Application Support/JobsIM/Regression/20261117-Canonical-完整运行');
const ca = fs.readFileSync(path.join(state, 'TLS/ca.crt'));
const certificate = new crypto.X509Certificate(fs.readFileSync(path.join(state, 'TLS/server.crt')));
const spki = crypto.createHash('sha256').update(certificate.publicKey.export({ type: 'spki', format: 'der' })).digest('base64');
const out = path.join(__dirname, Date.now() + '-' + process.pid);
const free = () => { const d = fs.statfsSync(root); return Number(d.bavail) * Number(d.bsize); };
if (free() < 4 * 1024 ** 3) throw Error('DISK_GATE');
fs.mkdirSync(out, { recursive: false });
const hash = data => crypto.createHash('sha256').update(data).digest('hex');
const sourceFiles = fs.readdirSync(path.join(root, '代码/前端/Web/Src')).filter(n => /\.(ts|tsx|css)$/.test(n)).map(n => '代码/前端/Web/Src/' + n).concat(['index.html', 'package.json', 'package-lock.json', 'tsconfig.json', 'vite.config.ts'].map(n => '代码/前端/Web/' + n));
const sources = () => Object.fromEntries(sourceFiles.map(n => [n, hash(fs.readFileSync(path.join(root, n)))]));
const before = sources(), checks = [], network = [], pageErrors = [], startedAt = new Date().toISOString();
const username = 'whp_' + Date.now().toString(36) + crypto.randomBytes(3).toString('hex');
let oldPassword = 'Old!' + crypto.randomBytes(24).toString('base64url'), newPassword = 'New!' + crypto.randomBytes(24).toString('base64url');
let browser, context, page, minimumFreeBytes = free(), originalCookie = '', currentCookie = '', originalFamily = '', successorFamily = '';
const assert = (condition, reason) => { if (!condition) throw Error(reason); };
const strict = (route, method = 'GET', body, headers = {}) => new Promise((resolve, reject) => {
  const data = body === undefined ? undefined : Buffer.from(JSON.stringify(body));
  const req = https.request(new URL(route, origin), { method, ca, agent: false, rejectUnauthorized: true, timeout: 15000,
    headers: { 'X-JobsIM-Client': 'web', Origin: origin, ...(data ? { 'Content-Type': 'application/json', 'Content-Length': data.length } : {}), ...headers } }, res => {
    const chunks = []; let length = 0;
    res.on('data', chunk => { length += chunk.length; if (length > 4 * 1024 * 1024) req.destroy(Error('BOUNDED_RESPONSE')); else chunks.push(chunk); });
    res.on('end', () => resolve({ status: res.statusCode, body: Buffer.concat(chunks), headers: res.headers }));
  });
  req.on('socket', socket => socket.on('secureConnect', () => {
    try { assert(socket.authorized, 'STRICT_TLS_NOT_AUTHORIZED'); assert(new crypto.X509Certificate(socket.getPeerCertificate().raw).fingerprint256 === certificate.fingerprint256, 'TLS_LEAF_CHANGED'); }
    catch (error) { req.destroy(error); }
  }));
  req.on('timeout', () => req.destroy(Error('STRICT_TLS_TIMEOUT'))); req.on('error', reject); req.end(data);
});
const cookie = async () => (await context.cookies(origin)).map(c => c.name + '=' + c.value).join('; ');
const screenshot = async name => {
  assert(await page.locator('input[type=password]').evaluateAll(inputs => inputs.every(input => input.value === '')), 'SCREENSHOT_CONTAINS_PASSWORD_INPUT');
  await page.screenshot({ path: path.join(out, name + '.png'), fullPage: true });
};
const authResponse = async response => {
  const value = await response.json();
  return { status: response.status(), family: value.familyId, user: value.user?.id, administrator: value.user?.admin === true, code: value.error?.code };
};
const openLogin = async () => {
  if (!await page.locator('.auth-modal').isVisible()) await page.locator('.me-avatar').click();
  await page.locator('.auth-modal input[name=username]').waitFor({ state: 'visible' });
};
const login = async (password, expectSuccess) => {
  await openLogin();
  await page.locator('.auth-modal input[name=username]').fill(username);
  await page.locator('.auth-modal input[name=password]').fill(password);
  const pending = page.waitForResponse(r => /^\/api\/auth\/(login|reauthenticate)$/.test(new URL(r.url()).pathname) && r.request().method() === 'POST', { timeout: 25000 });
  const fresh = page.locator('.auth-modal button[value=fresh-proof]');
  if (await fresh.isVisible()) await fresh.click(); else await page.locator('.auth-modal form button.primary').click();
  const result = await authResponse(await pending);
  if (expectSuccess) {
    assert(result.status === 200 && !!result.family && !result.administrator, 'UI_LOGIN_FAILED');
    await page.locator('.auth-modal').waitFor({ state: 'detached', timeout: 25000 });
  } else {
    assert([401, 403].includes(result.status), 'OLD_PASSWORD_NOT_REJECTED');
    await page.locator('.auth-modal input[name=password]').fill('');
  }
  return result;
};
const settings = async () => {
  await page.locator('.rail-bottom > button.icon-button').click();
  await page.locator('.password-change-panel').waitFor({ state: 'visible' });
  await page.locator('.settings-content select').first().selectOption('en');
  await page.getByRole('button', { name: 'Settings', exact: true }).waitFor({ state: 'visible' });
};
const logout = async () => {
  const pending = page.waitForResponse(r => new URL(r.url()).pathname === '/api/auth/logout' && r.request().method() === 'POST');
  await page.locator('.settings-content > button.secondary').click();
  assert((await pending).status() === 200, 'OWN_UI_LOGOUT_NOT_CONFIRMED');
  await page.waitForFunction(() => document.querySelector('.settings-content > button.secondary')?.textContent?.trim() === 'Sign in');
};
const timer = setInterval(() => {
  minimumFreeBytes = Math.min(minimumFreeBytes, free());
  if (minimumFreeBytes < 4 * 1024 ** 3) { console.error('DISK_GATE'); process.exit(91); }
}, 500);
(async () => {
  let failure;
  const runtime = {};
  try {
    const connection = JSON.parse(fs.readFileSync(path.join(state, 'connection.json'), 'utf8'));
    const preflight = await strict('/api/bootstrap');
    assert(preflight.status === 200, 'BOOTSTRAP_NOT_200');
    const bootstrap = JSON.parse(preflight.body);
    assert(bootstrap.features?.own_password_change === true && bootstrap.features?.auth_refresh === true, 'REAL_PASSWORD_PROTOCOL_UNAVAILABLE');
    runtime.deploymentId = bootstrap.deploymentId; runtime.restoreEpoch = bootstrap.restoreEpoch;
    assert(typeof runtime.deploymentId === 'string' && runtime.deploymentId.startsWith('3334f58a'), 'NOT_OWN_CANONICAL_REALM');
    runtime.connectionDeploymentMatches = !connection.deploymentId || connection.deploymentId === runtime.deploymentId;
    assert(runtime.connectionDeploymentMatches, 'CONNECTION_INSTANCE_MISMATCH');
    const home = await strict('/'); assert(home.status === 200, 'HOME_NOT_200');
    const script = home.body.toString().match(/src="([^"]*index-B3U1AnNU\.js)"/);
    assert(script, 'NOT_FROZEN_B3_PRODUCTION');
    const bundle = await strict(script[1]); assert(bundle.status === 200, 'BUNDLE_NOT_200');
    const handoff = JSON.parse(fs.readFileSync(path.join(__dirname, '../Web逐存储收讫/15-当前输入与产物交接.json')));
    assert(hash(bundle.body) === handoff.artifactSHA256['assets/index-B3U1AnNU.js'], 'SERVED_BUNDLE_HASH_MISMATCH');
    runtime.homeSHA256 = hash(home.body); runtime.bundle = script[1]; runtime.bundleSHA256 = hash(bundle.body);
    checks.push('Strict Node CA/host verification and active leaf match; served B3 JavaScript equals frozen production SHA');
    browser = await chromium.launch({ headless: true, executablePath: '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome', args: ['--ignore-certificate-errors-spki-list=' + spki] });
    context = await browser.newContext({ viewport: { width: 1280, height: 900 }, ignoreHTTPSErrors: false });
    await context.addInitScript(() => { localStorage.setItem('jobsim.locale', 'en'); localStorage.setItem('jobsim.theme', 'light'); });
    page = await context.newPage(); page.setDefaultTimeout(15000);
    page.on('pageerror', e => pageErrors.push(e.name + ': ' + e.message));
    page.on('response', response => { const u = new URL(response.url()); if (u.origin === origin && u.pathname.startsWith('/api/')) network.push({ path: u.pathname, method: response.request().method(), status: response.status() }); });
    await page.route('**/*', route => new URL(route.request().url()).origin === origin ? route.continue() : route.abort());
    await page.goto(origin, { waitUntil: 'domcontentloaded' });
    await openLogin(); await page.locator('.auth-links button').first().click();
    await page.locator('.auth-modal input[name=displayName]').waitFor({ state: 'visible' });
    await page.locator('.auth-modal input[name=username]').fill(username);
    await page.locator('.auth-modal input[name=password]').fill(oldPassword);
    const registration = page.waitForResponse(r => new URL(r.url()).pathname === '/api/auth/register' && r.request().method() === 'POST');
    await page.locator('.auth-modal form button.primary').click();
    const registered = await authResponse(await registration);
    assert(registered.status === 200 && !registered.administrator, 'REGISTRATION_NOT_NORMAL_USER');
    await page.locator('.auth-modal').waitFor({ state: 'detached', timeout: 25000 });
    await settings(); await logout();
    const signed = await login(oldPassword, true); originalFamily = signed.family; originalCookie = await cookie();
    const own = await strict('/api/auth/context', 'GET', undefined, { Cookie: originalCookie, 'X-JobsIM-Auth-Family': originalFamily });
    assert(own.status === 200 && JSON.parse(own.body).familyId === originalFamily, 'ORIGINAL_WEB_FAMILY_NOT_CURRENT');
    checks.push('Fresh random nonadministrator registers; production UI signs out and signs in with the original password');
    await settings(); await screenshot('01-真实登录-改密空表单');
    const panel = page.locator('.password-change-panel');
    await panel.locator('input[name=currentPassword]').fill(oldPassword);
    await panel.locator('input[name=newPassword]').fill(newPassword);
    await panel.locator('input[name=confirmPassword]').fill(newPassword);
    await panel.locator('input[name=consent]').check();
    const changed = page.waitForResponse(r => new URL(r.url()).pathname === '/api/auth/password-change' && r.request().method() === 'POST', { timeout: 30000 });
    await panel.locator('button[value=create]').click();
    const changeResponse = await changed, changeValue = await changeResponse.json();
    assert(changeResponse.status() === 200 && changeValue.status === 'changed' && changeValue.requiresNewLogin === true, 'REAL_CHANGE_NOT_CONFIRMED');
    runtime.originalRequestId = changeValue.requestId;
    await page.locator('.auth-modal input[name=username]').waitFor({ state: 'visible', timeout: 15000 });
    const revoked = await strict('/api/auth/context', 'GET', undefined, { Cookie: originalCookie, 'X-JobsIM-Auth-Family': originalFamily });
    assert(revoked.status === 401, 'ORIGINAL_FAMILY_STILL_ACCEPTED');
    checks.push('Production settings form changes password over HTTPS and returns to sign-in; original Cookie family is rejected');
    await screenshot('02-已改密-回登录');
    const rejected = await login(oldPassword, false); runtime.oldPasswordRejectedCode = rejected.code;
    const successor = await login(newPassword, true); successorFamily = successor.family; currentCookie = await cookie();
    assert(successorFamily !== originalFamily, 'SUCCESSOR_FAMILY_NOT_NEW');
    checks.push('Real original password is rejected and replacement password signs in through the production form with a new family');
    await settings();
    const row = page.locator('.password-change-panel article').filter({ hasText: changeValue.requestId });
    await row.getByRole('button', { name: 'Select original request', exact: true }).click();
    await panel.locator('input[name=currentPassword]').fill(oldPassword);
    await panel.locator('input[name=newPassword]').fill(newPassword);
    await panel.locator('input[name=confirmPassword]').fill(newPassword);
    const writesBefore = network.filter(n => n.path === '/api/auth/password-change' && n.method === 'POST').length;
    const queryPending = page.waitForResponse(r => new URL(r.url()).pathname === '/api/auth/password-change-results' && r.request().method() === 'POST');
    await panel.locator('button[value=check]').click();
    const queryResponse = await queryPending, queryValue = await queryResponse.json(), queryHeaders = await queryResponse.request().allHeaders();
    assert(queryResponse.status() === 200 && queryValue.requestId === changeValue.requestId && queryValue.status === 'changed', 'ORIGINAL_RESULT_NOT_RECOVERED');
    assert(!queryHeaders.cookie && !queryHeaders['x-jobsim-auth-family'] && !queryHeaders['x-csrf-token'], 'ORIGINAL_QUERY_CARRIED_SUCCESSOR_CREDENTIALS');
    assert(network.filter(n => n.path === '/api/auth/password-change' && n.method === 'POST').length === writesBefore, 'ORIGINAL_QUERY_REPUBLISHED_CHANGE');
    await panel.locator('button[value=check]').waitFor({ state: 'visible' });
    const stillCurrent = await strict('/api/auth/context', 'GET', undefined, { Cookie: currentCookie, 'X-JobsIM-Auth-Family': successorFamily });
    assert(stillCurrent.status === 200 && JSON.parse(stillCurrent.body).familyId === successorFamily, 'LATE_ORIGINAL_QUERY_CLEARED_SUCCESSOR');
    checks.push('Original result UI uses anonymous same-request lookup, sends zero replacement write and preserves the successor family');
    const noSecrets = await page.evaluate(async ({ first, second }) => {
      const local = Object.values(localStorage).join('\n') + Object.values(sessionStorage).join('\n');
      const databases = await indexedDB.databases(); let found = local.includes(first) || local.includes(second);
      for (const info of databases) {
        const db = await new Promise((resolve, reject) => { const q = indexedDB.open(info.name); q.onsuccess = () => resolve(q.result); q.onerror = () => reject(q.error); });
        for (const store of Array.from(db.objectStoreNames)) {
          const rows = await new Promise((resolve, reject) => { const tx = db.transaction(store), q = tx.objectStore(store).getAll(); tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error); });
          const raw = JSON.stringify(rows); found ||= raw.includes(first) || raw.includes(second);
        }
        db.close();
      }
      return !found;
    }, { first: oldPassword, second: newPassword });
    assert(noSecrets, 'SECRET_PERSISTED');
    await screenshot('03-新登录-原结果只查');
    checks.push('Browser localStorage/sessionStorage/IndexedDB contain neither password; original proof fields clear before screenshots');
    await logout();
    const ended = await strict('/api/auth/context', 'GET', undefined, { Cookie: currentCookie, 'X-JobsIM-Auth-Family': successorFamily });
    assert(ended.status === 401, 'OWN_SUCCESSOR_LOGOUT_FAILED');
    checks.push('Own successor family signs out; no users or history are deleted');
  } catch (error) {
    failure = { name: error?.name, message: String(error?.message ?? error).replaceAll(oldPassword, '[REDACTED]').replaceAll(newPassword, '[REDACTED]') };
  } finally {
    clearInterval(timer);
    if (context) {
      try {
        const ownCookie = await cookie();
        const active = await strict('/api/auth/context', 'GET', undefined, { Cookie: ownCookie, 'X-JobsIM-Context-Learn': '1' });
        runtime.finalOwnContextStatus = active.status;
        if (active.status === 200) {
          const value = JSON.parse(active.body);
          assert(value.user?.username === username, 'CLEANUP_NOT_OWN_RANDOM_USER');
          const csrf = await strict('/api/auth/csrf', 'GET', undefined, { Cookie: ownCookie, 'X-JobsIM-Auth-Family': value.familyId });
          assert(csrf.status === 200, 'OWN_LOGOUT_CSRF_UNAVAILABLE');
          const result = await strict('/api/auth/logout', 'POST', {}, { Cookie: ownCookie, 'X-JobsIM-Auth-Family': value.familyId, 'X-CSRF-Token': JSON.parse(csrf.body).csrfToken });
          runtime.finalOwnLogoutStatus = result.status;
        }
      } catch (error) { runtime.finalOwnLogoutUnconfirmed = true; runtime.finalOwnLogoutError = { name: error?.name, code: error?.code }; }
    }
    if (context) await context.close().catch(() => {});
    if (browser) await browser.close().catch(() => {});
    oldPassword = ''; newPassword = ''; originalCookie = ''; currentCookie = '';
    const after = sources();
    const result = { startedAt, finishedAt: new Date().toISOString(), origin, username, runtime, checks, passed: checks.length,
      failure, pageErrors, network, minimumFreeBytes, tls: { caFingerprint256: new crypto.X509Certificate(ca).fingerprint256, leafFingerprint256: certificate.fingerprint256, leafSPKI: spki, strictNodeCAAndHost: true, browserOnlyExactSPKI: true, ignoreHTTPSErrors: false },
      sourceBefore: before, sourceAfter: after, inputsUnchanged: JSON.stringify(before) === JSON.stringify(after), driverSHA256: hash(fs.readFileSync(__filename)) };
    fs.writeFileSync(path.join(out, '结果.json'), JSON.stringify(result, null, 2) + '\n');
    console.log(JSON.stringify({ out, passed: checks.length, failure, inputsUnchanged: result.inputsUnchanged, pageErrors: pageErrors.length }));
    if (failure || !result.inputsUnchanged) process.exitCode = 1;
  }
})();
