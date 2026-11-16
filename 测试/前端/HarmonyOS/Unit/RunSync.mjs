// Created by Jobs on 2026年11月16日，星期一.
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import crypto from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '../../../..');
const clt = process.env.JOBS_IM_HARMONY_CLT_HOME || path.join(os.homedir(), 'Library/Application Support/JobsIM/Toolchains/HarmonyMirror/ArkTS-6.1.1.280/command-line-tools');
const require = createRequire(import.meta.url);
const ts = require(path.join(clt, 'sdk/default/openharmony/ets/build-tools/ets-loader/node_modules/typescript'));
const base = path.join(os.homedir(), 'Library/Application Support/JobsIM/Build/Harmony');
fs.mkdirSync(base, { recursive: true });
const work = fs.mkdtempSync(path.join(base, 'Authoritative-'));
const data = path.join(root, '代码/前端/HarmonyOS/entry/src/main/ets/Data');
const hashes = {};
for (const name of ['Models', 'SyncProtocol', 'SelfProtocol', 'SelfState', 'SelfText', 'SyncEngine', 'SyncStore', 'SyncText']) {
  const file = path.join(data, name + '.ets');
  const source = fs.readFileSync(file, 'utf8');
  hashes[path.relative(root, file)] = crypto.createHash('sha256').update(source).digest('hex');
  const result = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS }, reportDiagnostics: true, fileName: name + '.ts' });
  if ((result.diagnostics || []).some(item => item.category === ts.DiagnosticCategory.Error)) throw new Error(name + ' syntax failure');
  fs.writeFileSync(path.join(work, name + '.js'), result.outputText);
  fs.writeFileSync(path.join(work, name + '.ts'), source);
}
const pure = ['Models.ts', 'SyncProtocol.ts', 'SelfProtocol.ts', 'SyncEngine.ts', 'SyncText.ts'].map(name => path.join(work, name));
const program = ts.createProgram(pure, { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS, strict: true, skipLibCheck: true, noEmit: true });
const errors = ts.getPreEmitDiagnostics(program);
fs.writeFileSync(path.join(work, 'TypeDiagnostics.txt'), errors.map(item => ts.flattenDiagnosticMessageText(item.messageText, ' ')).join('\n'));
if (errors.length) { process.stderr.write(fs.readFileSync(path.join(work, 'TypeDiagnostics.txt'), 'utf8')); process.exitCode = 1; }
else {
  const result = spawnSync(process.execPath, ['--test', path.join(here, 'Sync.test.cjs')], {
    encoding: 'utf8', timeout: 90000,
    env: { ...process.env, JOBS_IM_SYNC_HOST_WORK: work, JOBS_IM_SYNC_SQL_BRIDGE: path.join(here, 'SyncSqlite.py'), JOBS_IM_SYNC_NATIVE_STORE: path.join(data, 'Store.ets') }
  });
  const output = (result.stdout || '') + (result.stderr || '');
  fs.writeFileSync(path.join(work, 'Results.txt'), output);
  process.stdout.write(output); process.exitCode = result.status === null ? 1 : result.status;
}
for (const file of ['Store.ets', 'NativeServices.ets', 'AuthCoordinator.ets']) hashes[path.relative(root, path.join(data, file))] = crypto.createHash('sha256').update(fs.readFileSync(path.join(data, file))).digest('hex');
for (const file of ['RunSync.mjs', 'Sync.test.cjs', 'SyncSqlite.py']) hashes[path.relative(root, path.join(here, file))] = crypto.createHash('sha256').update(fs.readFileSync(path.join(here, file))).digest('hex');
fs.writeFileSync(path.join(work, 'SourceManifest.json'), JSON.stringify({ native: false, database: 'Python SQLite adapter, not Harmony RDB NAPI', work, hashes }, null, 2));
process.stdout.write('Host-only work: ' + work + '\n');
