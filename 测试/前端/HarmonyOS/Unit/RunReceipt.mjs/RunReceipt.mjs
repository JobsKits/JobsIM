// Created by Jobs on 2026年11月16日，星期一.
// Execute current qualified receipt service under host adapters; never connects to JobsIM.
import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';
const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '../../../../..');
if (fs.statfsSync(root).bavail * fs.statfsSync(root).bsize < 4 * 1024 ** 3) throw Error('4 GiB disk gate');
const clt = process.env.JOBS_IM_HARMONY_CLT_HOME || path.join(os.homedir(), 'Library/Application Support/JobsIM/Toolchains/HarmonyMirror/ArkTS-6.1.1.280/command-line-tools');
const ts = createRequire(import.meta.url)(path.join(clt, 'sdk/default/openharmony/ets/build-tools/ets-loader/node_modules/typescript'));
const reports = path.join(root, '项目相关报告/实现进度/20261116-剩余能力闭环/普通迁移两端证据'); fs.mkdirSync(reports, { recursive: true });
const work = fs.mkdtempSync(path.join(reports, 'HarmonyHost-')); const hashes = {};
for (const name of ['NativeStorageBudget', 'Models','SyncProtocol','SelfProtocol','MigrationProtocol','MigrationStore','ReceiptProtocol','ReceiptStore','ReceiptService','ReceiptCoordinator','ReceiptCleanup','ReceiptText']) {
 const file = path.join(root, '代码/前端/HarmonyOS/entry/src/main/ets/Data', name + '.ets'); const text = fs.readFileSync(file, 'utf8');
 hashes[path.relative(root, file)] = crypto.createHash('sha256').update(text).digest('hex');
 fs.writeFileSync(path.join(work, name + '.ts'), text);
 fs.writeFileSync(path.join(work, name + '.js'), ts.transpileModule(text, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS } }).outputText);
}
const errors = ts.getPreEmitDiagnostics(ts.createProgram(['Models','SyncProtocol','SelfProtocol','MigrationProtocol','ReceiptProtocol','ReceiptText'].map(name=>path.join(work,name+'.ts')),{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS,strict:true,noEmit:true,skipLibCheck:true}));
for (const file of [fileURLToPath(import.meta.url), path.resolve(here, '../Receipt.test.cjs/Receipt.test.cjs'), path.resolve(here, '../SyncSqlite.py')]) hashes[path.relative(root, file)] = crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
fs.writeFileSync(path.join(work, '类型.log'), errors.map(e => ts.flattenDiagnosticMessageText(e.messageText, ' ')).join('\n'));
let status = 1;
if (errors.length) process.stdout.write(fs.readFileSync(path.join(work, '类型.log'), 'utf8'));
else {
 const run = spawnSync(process.execPath, ['--test', path.resolve(here, '../Receipt.test.cjs/Receipt.test.cjs')], { encoding: 'utf8', timeout: 30000, env: { ...process.env, JOBS_IM_RECEIPT_HOST: work, JOBS_IM_RECEIPT_SQLITE: path.resolve(here, '../SyncSqlite.py') } });
 const log = (run.stdout || '') + (run.stderr || ''); fs.writeFileSync(path.join(work, '原始结果.log'), log); process.stdout.write(log); status = run.status ?? 1;
}
const afterHashes=Object.fromEntries(Object.keys(hashes).map(file=>[file,crypto.createHash('sha256').update(fs.readFileSync(path.join(root,file))).digest('hex')]));
const inputUnchanged=Object.keys(hashes).every(file=>hashes[file]===afterHashes[file]);if(!inputUnchanged)status=1;
fs.writeFileSync(path.join(work, '结果.json'), JSON.stringify({ exitCode: status, typeErrors: errors.length, native: false, https: false, inputUnchanged, hashes,afterHashes }, null, 2));
process.stdout.write('Host evidence: ' + path.relative(root, work) + '\n'); process.exitCode = status;
