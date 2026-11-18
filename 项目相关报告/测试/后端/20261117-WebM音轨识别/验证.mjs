import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { spawn } from 'node:child_process';
import { finished } from 'node:stream/promises';
import { fileURLToPath } from 'node:url';

const report = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(report, '../../../..');
const support = path.join(os.homedir(), 'Library/Application Support/JobsIM');
const go = path.join(support, 'Toolchains/go1.27.4/go/bin/go');
const cache = path.join(support, 'Build/CLI/20261116-113743-192d48/Cache');
const runID = crypto.randomUUID();
const run = path.join(report, runID);
const source = path.join(run, '冻结源码');
const build = path.join(support, 'Build/WebM', runID);
const minimumFree = 4n * 1024n ** 3n;
let minimumObserved = 2n ** 63n;
const inputs = new Map();
const summary = { runID, scope: 'frozen pure WebM/BMFF/media rules and optional cmd compile; no PostgreSQL, service, browser or native execution', stages: [] };

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
try {
  disk();
  if (!fs.existsSync(go) || !fs.existsSync(cache)) throw new Error('existing Go toolchain or warm cache unavailable; downloads are disabled');
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
  summary.inputs = { backend: production.length, externalTests: tests.length, total: inputs.size };
  writeJSON(path.join(run, '输入SHA256.json'), Object.fromEntries(inputs));
  if (changed().length) throw new Error('input changed during freeze');
  const env = Object.fromEntries(Object.entries(process.env).filter(([key]) => !key.startsWith('JOBS_IM_') && !key.startsWith('PG')));
  Object.assign(env, { GOMAXPROCS: '2', GOPROXY: 'off', GOTOOLCHAIN: 'local', GOCACHE: cache });
  const pattern = '^Test(WebM|AvatarDecodeAndCanonicalOutputs$|AvatarFilePublishIsIdempotentAndNeverOverwrites$|MediaRetentionIndependentModesRejectImplicitDelay$|MediaRetentionStableTimestampAndStreamingDeadline$)';
  await execute('01-媒体纯规则', ['test', '-json', '-p=1', '-count=1', '-timeout=120s', '-run', pattern, '.'], 150000, env);
  fs.mkdirSync(build, { recursive: true, mode: 0o700 });
  await execute('02-两个cmd编译', ['build', '-p=1', '-o', build + path.sep, './cmd/jobsimd', './cmd/jobsimops'], 120000, env);
  summary.passed = true;
} catch (error) {
  summary.error = error.message;
  process.stdout.write(JSON.stringify({ error: error.message }) + '\n');
} finally {
  summary.changedLiveInputs = changed();
  summary.minimumFreeBytes = minimumObserved.toString();
  summary.finishedAt = new Date().toISOString();
  writeJSON(path.join(run, '结果.json'), summary);
  process.stdout.write(JSON.stringify({ report: path.relative(root, run), passed: summary.passed ?? false, changedLiveInputs: summary.changedLiveInputs, minimumFreeBytes: summary.minimumFreeBytes }) + '\n');
  if (!summary.passed) process.exitCode = 1;
}
