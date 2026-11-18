import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn, spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const report = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(report, '../../../..');
const home = os.homedir();
const support = path.join(home, 'Library/Application Support/JobsIM');
const pgBin = path.join(support, 'Toolchains/PostgresApp-2.9.7/Postgres.app/Contents/Versions/18/bin');
const go = path.join(support, 'Toolchains/go1.27.4/go/bin/go');
const runID = crypto.randomUUID();
const run = path.join(report, runID);
const privateRoot = path.join(support, 'Integration', runID, 'Safety');
const data = path.join(privateRoot, 'Postgres/Main');
const secrets = path.join(privateRoot, 'Secrets');
const logs = path.join(privateRoot, 'Logs');
const owner = 's2_owner_' + runID.replaceAll('-', '').slice(0, 12);
const minimumFree = 4n * 1024n ** 3n;
let minimumObserved = 2n ** 63n;
let postgresStarted = false;
const source = path.join(run, '修后源码');
const beforeSource = path.join(run, '修前源码');
const originals = new Map();
const summary = { runID, scope: 'isolated PostgreSQL with in-process role RPC, no HTTPS/native/live-instance verification', stages: [] };

function disk() {
  const info = fs.statfsSync(home, { bigint: true });
  const available = info.bavail * info.bsize;
  if (available < minimumObserved) minimumObserved = available;
  if (available < minimumFree) throw new Error('4 GiB disk gate');
  return available;
}
function digest(bytes) { return crypto.createHash('sha256').update(bytes).digest('hex'); }
function writeJSON(file, value) { fs.writeFileSync(file, JSON.stringify(value, null, 2) + '\n'); }
function command(file, args, label, env = process.env) {
  const result = spawnSync(file, args, { env, encoding: 'utf8', timeout: 60000, maxBuffer: 1024 * 1024 });
  fs.writeFileSync(path.join(run, label + '.log'), (result.stdout || '') + (result.stderr || ''));
  if (result.status !== 0 || result.error) throw new Error(label + ' failed; see retained log');
  return result.stdout || '';
}
function walk(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const file = path.join(directory, entry.name);
    return entry.isDirectory() && !['.git', 'vendor', 'Vendor'].includes(entry.name) ? walk(file) : entry.isFile() ? [file] : [];
  });
}
function capture(file, destination) {
  const bytes = fs.readFileSync(file);
  fs.mkdirSync(path.dirname(destination), { recursive: true });
  fs.writeFileSync(destination, bytes);
  originals.set(path.relative(root, file), digest(bytes));
}
function changedInputs() {
  return [...originals].filter(([relative, sha]) => !fs.existsSync(path.join(root, relative)) || digest(fs.readFileSync(path.join(root, relative))) !== sha).map(([relative]) => relative);
}
async function test(directory, label, pattern, env) {
  disk();
  const args = ['test', '-json', '-p=1', '-count=1', '-timeout=180s', '-run', pattern, '.'];
  const out = fs.createWriteStream(path.join(run, label + '.jsonl'));
  const err = fs.createWriteStream(path.join(run, label + '.stderr.log'));
  const child = spawn(go, args, { cwd: directory, env, detached: true, stdio: ['ignore', 'pipe', 'pipe'] });
  child.stdout.pipe(out);
  child.stderr.pipe(err);
  let gate = null;
  const stop = reason => {
    if (gate) return;
    gate = reason;
    try { process.kill(-child.pid, 'SIGTERM'); } catch {}
  };
  const monitor = setInterval(() => { try { disk(); } catch { stop('4 GiB disk gate'); } }, 500);
  const deadline = setTimeout(() => stop('210-second process bound'), 210000);
  const result = await new Promise((resolve, reject) => {
    child.on('error', reject);
    child.on('close', (code, signal) => resolve({ code, signal }));
  });
  clearInterval(monitor);
  clearTimeout(deadline);
  await Promise.all([new Promise(resolve => out.end(resolve)), new Promise(resolve => err.end(resolve))]);
  const events = fs.readFileSync(path.join(run, label + '.jsonl'), 'utf8').split('\n').filter(Boolean).map(line => { try { return JSON.parse(line); } catch { return {}; } });
  const stage = { label, command: ['go', ...args], ...result, gate, pass: events.filter(e => e.Action === 'pass' && e.Test && !e.Test.includes('/')).map(e => e.Test), skip: events.filter(e => e.Action === 'skip' && e.Test && !e.Test.includes('/')).map(e => e.Test), fail: events.filter(e => e.Action === 'fail' && e.Test).map(e => e.Test) };
  summary.stages.push(stage);
  writeJSON(path.join(run, '结果.json'), summary);
  process.stdout.write(JSON.stringify(stage) + '\n');
  if (gate) throw new Error(gate);
  return stage;
}

fs.mkdirSync(run, { recursive: true });
try {
  disk();
  const backend = path.join(root, '代码/后端/Go');
  for (const file of walk(backend).filter(file => file.endsWith('.go') || ['go.mod', 'go.sum'].includes(path.basename(file)))) capture(file, path.join(source, path.relative(backend, file)));
  const tests = walk(path.join(root, '测试/后端/接口')).filter(file => file.endsWith('_test.go'));
  const names = new Set();
  for (const file of tests) {
    const name = path.basename(file);
    if (names.has(name)) throw new Error('duplicate external test filename');
    names.add(name);
    capture(file, path.join(source, name));
  }
  writeJSON(path.join(run, '输入SHA256.json'), Object.fromEntries(originals));
  if (changedInputs().length) throw new Error('source changed during capture');
  fs.cpSync(source, beforeSource, { recursive: true });
  for (const file of ['account_deletion.go', 'account_deletion_governance.go', 'account_deletion_proof.go']) fs.copyFileSync(path.join(report, '修改前-' + file), path.join(beforeSource, file));
  writeJSON(path.join(run, '修前替换SHA256.json'), Object.fromEntries(['account_deletion.go', 'account_deletion_governance.go', 'account_deletion_proof.go'].map(file => [file, digest(fs.readFileSync(path.join(beforeSource, file)))])));

  for (const directory of [secrets, logs, path.dirname(data)]) fs.mkdirSync(directory, { recursive: true, mode: 0o700 });
  const password = crypto.randomBytes(32).toString('hex');
  const passwordFile = path.join(secrets, 'init-password');
  const passfile = path.join(secrets, 'pgpass');
  fs.writeFileSync(passwordFile, password + '\n', { mode: 0o600, flag: 'wx' });
  fs.writeFileSync(passfile, `127.0.0.1:62381:*:${owner}:${password}\n`, { mode: 0o600, flag: 'wx' });
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('JOBS_IM_') && !key.startsWith('PG')));
  Object.assign(env, { PGPASSFILE: passfile, PGCONNECT_TIMEOUT: '3', GOMAXPROCS: '2', GOPROXY: 'off', GOTOOLCHAIN: 'local', GOCACHE: path.join(support, 'Build/CLI/20261116-113743-192d48/Cache'), JOBS_IM_SYNC_SCHEMAS_TEST: '1', JOBS_IM_SYNC_SCHEMAS_REALM_URL: 'https://127.0.0.1:56443', JOBS_IM_SYNC_SCHEMAS_ADMIN_URL: `postgresql://${owner}@127.0.0.1:62381/postgres?sslmode=disable` });
  command(path.join(pgBin, 'initdb'), ['-D', data, '-U', owner, '--encoding=UTF8', '--locale=C', '--auth-local=scram-sha-256', '--auth-host=scram-sha-256', '--pwfile=' + passwordFile, '--no-instructions'], '01-initdb', env);
  disk();
  command(path.join(pgBin, 'pg_ctl'), ['-D', data, '-l', path.join(logs, 'postgres.log'), '-o', "-h 127.0.0.1 -p 62381 -k '' -c shared_buffers=32MB -c max_connections=20 -c min_wal_size=32MB -c max_wal_size=64MB", '-w', 'start'], '02-start', env);
  postgresStarted = true;
  const identity = command(path.join(pgBin, 'psql'), ['-h', '127.0.0.1', '-p', '62381', '-U', owner, '-d', 'postgres', '-XAt', '-c', "SELECT system_identifier,current_setting('data_directory'),current_setting('fsync'),current_setting('synchronous_commit') FROM pg_control_system()"], '03-owned-cluster', env);
  summary.cluster = { identity: identity.trim(), port: 62381, newPrivateRun: true, externalTests: tests.length };
  await test(beforeSource, '04-修前反例', '^TestClosureAuditDeletion', env);
  const green = await test(source, '05-修后注销回归', '^Test(ClosureAuditDeletion|AccountDeletion)', env);
  summary.greenPassed = green.code === 0 && green.fail.length === 0 && green.skip.length === 0;
} catch (error) {
  summary.error = error.message;
  process.stdout.write(JSON.stringify({ error: error.message }) + '\n');
} finally {
  if (postgresStarted) {
    try { command(path.join(pgBin, 'pg_ctl'), ['-D', data, '-m', 'fast', '-w', 'stop'], '06-stop'); summary.ownedClusterStopped = true; }
    catch (error) { summary.stopError = error.message; }
  }
  summary.minimumFreeBytes = minimumObserved.toString();
  summary.changedLiveInputs = changedInputs();
  summary.finishedAt = new Date().toISOString();
  writeJSON(path.join(run, '结果.json'), summary);
  process.stdout.write(JSON.stringify({ report: path.relative(root, run), greenPassed: summary.greenPassed ?? false, minimumFreeBytes: summary.minimumFreeBytes, changedLiveInputs: summary.changedLiveInputs, ownedClusterStopped: summary.ownedClusterStopped ?? false }) + '\n');
  if (!summary.greenPassed || summary.stopError) process.exitCode = 1;
}
