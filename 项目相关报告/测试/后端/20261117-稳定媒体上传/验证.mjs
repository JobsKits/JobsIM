import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { finished } from 'node:stream/promises';
import { fileURLToPath } from 'node:url';

const report = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(report, '../../../..');
const support = path.join(os.homedir(), 'Library/Application Support/JobsIM');
const go = path.join(support, 'Toolchains/go1.27.4/go/bin/go');
const pg = path.join(support, 'Toolchains/PostgresApp-2.9.7/Postgres.app/Contents/Versions/18/bin');
const cache = path.join(support, 'Build/CLI/20261116-113743-192d48/Cache');
const ownedRun = 'a199e288-d70b-4682-878e-3fbecc4f5a1b';
const privateRoot = path.join(support, 'Integration', ownedRun, 'Safety');
const data = path.join(privateRoot, 'Postgres/Main');
const passfile = path.join(privateRoot, 'Secrets/pgpass');
const priorIdentity = path.join(root, '项目相关报告/测试/后端/20261117-注销续跑公平性', ownedRun, '03-owned-cluster.log');
const browser = path.join(root, '项目相关报告/测试/前端/Web/20261117-Web基础音频/03-原生浏览器-1794882303104-90890/浏览器/原生合成音轨.webm');
const browserSHA = '3c7890c6485fcb0d914faf0c0449ae037619cdf059b36387e56f642e22b7edef';
const runID = crypto.randomUUID();
const run = path.join(report, runID);
const source = path.join(run, '冻结源码');
const minimumFree = 4n * 1024n ** 3n;
let minimumObserved = 2n ** 63n;
let ownedStarted = false;
const inputs = new Map();
const summary = { runID, scope: 'frozen pure rules plus owned isolated PostgreSQL production-handler fixtures; in-process role RPC, no real HTTP/NATS/native/microphone', stages: [] };

function disk() {
  const stat = fs.statfsSync(os.homedir(), { bigint: true });
  const available = stat.bavail * stat.bsize;
  if (available < minimumObserved) minimumObserved = available;
  if (available < minimumFree) throw new Error('4 GiB disk gate');
}
function digest(bytes) { return crypto.createHash('sha256').update(bytes).digest('hex'); }
function writeJSON(file, value) { fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n'); }
function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const file = path.join(directory, entry.name);
    return entry.isDirectory() && !['.git', 'vendor', 'Vendor', 'node_modules', 'build'].includes(entry.name) ? walk(file) : entry.isFile() ? [file] : [];
  });
}
function capture(file, destination) {
  const bytes = fs.readFileSync(file);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.writeFileSync(destination, bytes);
  inputs.set(path.relative(root, file), digest(bytes));
}
function changed() {
  return [...inputs].filter(([relative, sha]) => !fs.existsSync(path.join(root, relative)) || digest(fs.readFileSync(path.join(root, relative))) !== sha).map(([relative]) => relative);
}
function command(file, args, label, env) {
  const result = spawnSync(file, args, { env, encoding: 'utf8', timeout: 60000, maxBuffer: 1024 * 1024 });
  fs.writeFileSync(path.join(run, label + '.log'), (result.stdout || '') + (result.stderr || ''));
  if (result.status !== 0 || result.error) throw new Error(label + ' failed; see retained original');
  return result.stdout || '';
}
async function execute(label, args, timeout, env) {
  disk();
  const out = fs.createWriteStream(path.join(run, label + '.jsonl'));
  const err = fs.createWriteStream(path.join(run, label + '.stderr.log'));
  const child = spawn(go, args, { cwd: source, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
  child.stdout.pipe(out);
  child.stderr.pipe(err);
  let gate = null;
  const stop = reason => {
    if (gate) return;
    gate = reason;
    try { process.kill(-child.pid, 'SIGTERM'); } catch {}
  };
  const monitor = setInterval(() => { try { disk(); } catch { stop('4 GiB disk gate'); } }, 500);
  const deadline = setTimeout(() => stop('bounded process timeout'), timeout);
  let status;
  try {
    status = await new Promise((resolve, reject) => {
      child.on('error', reject);
      child.on('close', (code, signal) => resolve({ code, signal }));
    });
    await Promise.all([finished(out), finished(err)]);
  } finally {
    clearInterval(monitor);
    clearTimeout(deadline);
  }
  const events = fs.readFileSync(path.join(run, label + '.jsonl'), 'utf8').split('\n').filter(Boolean).map(line => { try { return JSON.parse(line); } catch { return {}; } });
  const stage = { label, command: ['go', ...args], ...status, gate, pass: events.filter(e => e.Action === 'pass' && e.Test && !e.Test.includes('/')).map(e => e.Test), passEvents: events.filter(e => e.Action === 'pass' && e.Test).length, skip: events.filter(e => e.Action === 'skip' && e.Test).map(e => e.Test), fail: events.filter(e => e.Action === 'fail' && e.Test).map(e => e.Test) };
  summary.stages.push(stage);
  writeJSON(path.join(run, '结果.json'), summary);
  process.stdout.write(JSON.stringify(stage) + '\n');
  if (gate || stage.code !== 0 || stage.fail.length || stage.skip.length) throw new Error(label + ' did not pass');
}

fs.mkdirSync(run, { recursive: true });
const baseEnv = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('JOBS_IM_') && !key.startsWith('PG')));
Object.assign(baseEnv, { GOMAXPROCS: '2', GOPROXY: 'off', GOTOOLCHAIN: 'local', GOCACHE: cache, LC_ALL: 'C' });
try {
  disk();
  const backend = path.join(root, '代码/后端/Go');
  const production = walk(backend).filter(file => file.endsWith('.go') || ['go.mod', 'go.sum'].includes(path.basename(file)));
  for (const file of production) capture(file, path.join(source, path.relative(backend, file)));
  const tests = walk(path.join(root, '测试/后端')).filter(file => file.endsWith('_test.go'));
  const names = new Set();
  for (const file of tests) {
    const name = path.basename(file);
    if (names.has(name) || fs.existsSync(path.join(source, name))) throw new Error('duplicate external test basename: ' + name);
    names.add(name);
    capture(file, path.join(source, name));
  }
  if (digest(fs.readFileSync(browser)) !== browserSHA) throw new Error('browser original SHA differs');
  const frozenBrowser = path.join(run, '真实浏览器合成音轨.webm');
  capture(browser, frozenBrowser);
  summary.inputs = { backend: production.length, externalTests: tests.length, total: inputs.size };
  summary.browser = { source: path.relative(root, browser), sha256: browserSHA, syntheticAudioTrack: true, realMicrophone: false };
  writeJSON(path.join(run, '输入SHA256.json'), Object.fromEntries(inputs));
  if (changed().length) throw new Error('input changed during freeze');
  const pureEnv = { ...baseEnv, JOBS_IM_WEBM_BROWSER_FIXTURE: frozenBrowser, JOBS_IM_WEBM_BROWSER_SHA256: browserSHA };
  await execute('01-纯规则与真实容器', ['test', '-json', '-p=1', '-count=1', '-timeout=120s', '-run', '^Test(WebM|MediaUploadManifestAndMinimalSafetyFields$)', '.'], 150000, pureEnv);

  disk();
  if ((fs.statSync(passfile).mode & 0o777) !== 0o600) throw new Error('private passfile mode must be 0600');
  const fields = fs.readFileSync(passfile, 'utf8').trim().split(':');
  const owner = fields[3];
  if (fields[0] !== '127.0.0.1' || fields[1] !== '62381' || owner !== 's2_owner_' + ownedRun.replaceAll('-', '').slice(0, 12)) throw new Error('private owner/port binding differs');
  const expected = fs.readFileSync(priorIdentity, 'utf8').trim().split('|');
  if (expected[1] !== data) throw new Error('prior exact -D differs');
  const control = command(path.join(pg, 'pg_controldata'), ['-D', data], '02-owned-control', baseEnv);
  const identifier = /Database system identifier:\s*(\d+)/.exec(control)?.[1];
  if (identifier !== expected[0]) throw new Error('owned system identifier differs');
  const status = spawnSync(path.join(pg, 'pg_ctl'), ['-D', data, 'status'], { env: baseEnv, encoding: 'utf8', timeout: 10000 });
  fs.writeFileSync(path.join(run, '03-before-start.log'), (status.stdout || '') + (status.stderr || ''));
  if (status.status !== 3) throw new Error('owned cluster is not stopped; no existing process will be taken over');
  const env = { ...pureEnv, PGPASSFILE: passfile, PGCONNECT_TIMEOUT: '3', JOBS_IM_SYNC_SCHEMAS_TEST: '1', JOBS_IM_SYNC_SCHEMAS_REALM_URL: 'https://127.0.0.1:56443', JOBS_IM_SYNC_SCHEMAS_ADMIN_URL: `postgresql://${owner}@127.0.0.1:62381/postgres?sslmode=disable` };
  ownedStarted = true;
  command(path.join(pg, 'pg_ctl'), ['-D', data, '-l', path.join(privateRoot, 'Logs', 'media-upload-' + runID + '.log'), '-o', "-h 127.0.0.1 -p 62381 -k '' -c shared_buffers=32MB -c max_connections=20 -c min_wal_size=32MB -c max_wal_size=64MB", '-w', 'start'], '04-owned-start', env);
  const live = command(path.join(pg, 'psql'), ['-h', '127.0.0.1', '-p', '62381', '-U', owner, '-d', 'postgres', '-XAt', '-c', "SELECT system_identifier,current_setting('data_directory'),current_setting('fsync'),current_setting('synchronous_commit') FROM pg_control_system()"], '05-owned-identity', env).trim().split('|');
  if (live[0] !== expected[0] || live[1] !== data || live[2] !== 'on' || live[3] !== 'on') throw new Error('live owned identity or durability differs');
  summary.cluster = { port: 62381, systemIdentifier: live[0], sameOwnedDataDirectory: true, fsync: live[2], synchronousCommit: live[3], privatePassfileMode: '0600' };
  await execute('06-上传与注销PG', ['test', '-json', '-p=1', '-count=1', '-timeout=240s', '-run', '^Test(MediaUploadPG|AccountDeletion|ClosureAuditDeletion)', '.'], 270000, env);
  command(path.join(pg, 'pg_ctl'), ['-D', data, '-m', 'fast', '-w', 'stop'], '07-owned-stop', baseEnv);
  ownedStarted = false;
  summary.ownedClusterStopped = true;
  const build = path.join(support, 'Build/MediaUpload', runID);
  fs.mkdirSync(build, { recursive: true, mode: 0o700 });
  await execute('08-两个cmd编译', ['build', '-p=1', '-o', build + path.sep, './cmd/jobsimd', './cmd/jobsimops'], 120000, baseEnv);
  summary.passed = true;
} catch (error) {
  summary.error = error.message;
  process.stdout.write(JSON.stringify({ error: error.message }) + '\n');
} finally {
  if (ownedStarted) {
    try { command(path.join(pg, 'pg_ctl'), ['-D', data, '-m', 'fast', '-w', 'stop'], '09-final-owned-stop', baseEnv); summary.ownedClusterStopped = true; }
    catch (error) { summary.stopError = error.message; }
  }
  summary.changedLiveInputs = changed();
  summary.minimumFreeBytes = minimumObserved.toString();
  summary.finishedAt = new Date().toISOString();
  writeJSON(path.join(run, '结果.json'), summary);
  process.stdout.write(JSON.stringify({ report: path.relative(root, run), passed: summary.passed ?? false, changedLiveInputs: summary.changedLiveInputs, minimumFreeBytes: summary.minimumFreeBytes, ownedClusterStopped: summary.ownedClusterStopped ?? false }) + '\n');
  if (!summary.passed || summary.stopError) process.exitCode = 1;
}
