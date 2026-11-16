#!/usr/bin/env node
// Overlay keeps external tests outside the production module.
import { createHash, randomBytes } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../../../', import.meta.url));
const disk = fs.statfsSync(root);
if (disk.bavail * disk.bsize < 4 * 1024 ** 3) throw new Error('Less than 4 GiB available; no compiler or database connection started.');
let go = process.env.GO_BINARY || 'go';
let withPG = false;
const args = process.argv.slice(2);
for (let i = 0; i < args.length; i++) {
  if (args[i] === '--go' && args[i + 1]) go = args[++i];
  else if (args[i] === '--with-pg') withPG = true;
  else throw new Error('Usage: node password_change.mjs [--go EXECUTABLE] [--with-pg]');
}
if (withPG && process.env.JOBS_IM_SYNC_SCHEMAS_TEST !== '1') throw new Error('Explicit isolated PostgreSQL test environment required.');
const stamp = new Date().toISOString().replaceAll(/[:.]/g, '-') + '-' + randomBytes(3).toString('hex');
const output = path.join(root, '项目相关报告/测试/后端/20261116-本人主动改密/执行记录', stamp);
fs.mkdirSync(output, { recursive: true });
const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'jobsim-password-tests-'));
const names = ['password_change_test.go', 'password_change_pg_test.go', 'sync_register_pg_test.go', 'sync_schemas_test.go', 's2_pg_guard_test.go'];
const replacements = Object.fromEntries(names.map(name => [path.join(root, '代码/后端/Go', name), path.join(root, '测试/后端/接口', name)]));
const overlay = path.join(temporary, 'overlay.json');
fs.writeFileSync(overlay, JSON.stringify({ Replace: replacements }));
function sources(directory) {
  return fs.readdirSync(directory, { withFileTypes: true }).flatMap(entry => {
    const file = path.join(directory, entry.name);
    if (entry.isDirectory() && !['Vendor', 'vendor', 'node_modules', 'build'].includes(entry.name)) return sources(file);
    return entry.isFile() && entry.name.endsWith('.go') ? [file] : [];
  });
}
const inputs = [...sources(path.join(root, '代码/后端/Go')), ...Object.values(replacements), path.join(root, '代码/后端/Go/go.mod'), path.join(root, '代码/后端/Go/go.sum'), fileURLToPath(import.meta.url)];
const digest = file => createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const before = Object.fromEntries(inputs.map(file => [path.relative(root, file), digest(file)]));
fs.writeFileSync(path.join(output, '输入SHA256.json'), JSON.stringify(before, null, 2) + '\n');
const environment = { ...process.env, GOMAXPROCS: '2', GOPROXY: 'off', GOTOOLCHAIN: 'local' };
if (!withPG) delete environment.JOBS_IM_SYNC_SCHEMAS_TEST;
const results = [];
function run(name, selection) {
  const began = Date.now();
  const result = spawnSync(go, ['test', '-p', '1', '-count=1', '-json', '-overlay', overlay, '-run', selection, '-timeout', '150s', '.'], { cwd: path.join(root, '代码/后端/Go'), env: environment, encoding: 'utf8', timeout: 180000, maxBuffer: 16 << 20 });
  fs.writeFileSync(path.join(output, name + '.jsonl'), result.stdout || '');
  fs.writeFileSync(path.join(output, name + '.stderr.log'), result.stderr || '');
  const events = (result.stdout || '').split('\n').filter(Boolean).map(line => { try { return JSON.parse(line); } catch { return {}; } });
  const record = { name, exitCode: result.status, signal: result.signal, elapsedSeconds: (Date.now() - began) / 1000, passed: events.filter(v => v.Test && v.Action === 'pass').length, failed: events.filter(v => v.Test && v.Action === 'fail').length, skipped: events.filter(v => v.Test && v.Action === 'skip').length };
  results.push(record);
  if (result.error || result.status !== 0 || name === '真实PG' && record.skipped > 0) throw new Error(name + ' failed; raw output retained at ' + output);
}
try {
  run('本地规则', '^TestPasswordChange(Decode|Proof|Sealed|Result)');
  if (withPG) run('真实PG', '^TestPasswordChangePG');
} finally {
  const changed = inputs.filter(file => digest(file) !== before[path.relative(root, file)]).map(file => path.relative(root, file));
  fs.writeFileSync(path.join(output, '结果.json'), JSON.stringify({ results, changedInputs: changed, withPG, authAndJournal: 'production handlers against an owned random PostgreSQL database', sourceDependencies: 'in-process acknowledgement fixtures', liveHTTPS: false, independentClusterRestore: false, nativeClients: false }, null, 2) + '\n');
  fs.rmSync(temporary, { recursive: true });
  console.log(output);
  if (changed.length) process.exitCode = 1;
}
