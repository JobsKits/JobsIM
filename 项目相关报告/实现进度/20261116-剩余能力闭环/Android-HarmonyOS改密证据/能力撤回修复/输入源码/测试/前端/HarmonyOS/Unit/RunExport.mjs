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
if (fs.statfsSync(root).bavail * fs.statfsSync(root).bsize < 4 * 1024 ** 3) throw Error('4 GiB disk gate');
const clt = process.env.JOBS_IM_HARMONY_CLT_HOME || path.join(os.homedir(), 'Library/Application Support/JobsIM/Toolchains/HarmonyMirror/ArkTS-6.1.1.280/command-line-tools');
const ts = createRequire(import.meta.url)(path.join(clt, 'sdk/default/openharmony/ets/build-tools/ets-loader/node_modules/typescript'));
const out = path.join(root, '项目相关报告/测试/前端/HarmonyOS/20261116-正式本人导出'); fs.mkdirSync(out, { recursive: true });
const work = fs.mkdtempSync(path.join(out, 'Host-')); const hashes = {};
for (const name of ['Models', 'SyncProtocol', 'SelfProtocol', 'ExportProtocol', 'ExportService', 'ExportStore', 'ExportFiles', 'ExportText', 'Api']) {
  const file = path.join(root, '代码/前端/HarmonyOS/entry/src/main/ets/Data', name + '.ets'); const text = fs.readFileSync(file, 'utf8');
  hashes[path.relative(root, file)] = crypto.createHash('sha256').update(text).digest('hex');
  const result = ts.transpileModule(text, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS }, reportDiagnostics: true, fileName: name + '.ts' });
  if ((result.diagnostics || []).some(row => row.category === ts.DiagnosticCategory.Error)) throw Error(name + ' syntax');
  fs.writeFileSync(path.join(work, name + '.js'), result.outputText); fs.writeFileSync(path.join(work, name + '.ts'), text);
}
const pure = ['Models', 'SyncProtocol', 'SelfProtocol', 'ExportProtocol', 'ExportService', 'ExportText'].map(name => path.join(work, name + '.ts'));
const diagnostics = ts.getPreEmitDiagnostics(ts.createProgram(pure, { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS, strict: true, noEmit: true, skipLibCheck: true }));
fs.writeFileSync(path.join(work, '类型检查.log'), diagnostics.map(row => ts.flattenDiagnosticMessageText(row.messageText, ' ')).join('\n'));
const inputs = ['代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets', '代码/前端/HarmonyOS/entry/src/main/ets/Data/AuthCoordinator.ets', '测试/前端/HarmonyOS/OhosTest/ets/test/List.test.ets', '测试/前端/HarmonyOS/Unit/RunExport.mjs', '测试/前端/HarmonyOS/Unit/Export.test.cjs', '测试/前端/HarmonyOS/Unit/SyncSqlite.py'];
for (const input of inputs) hashes[input] = crypto.createHash('sha256').update(fs.readFileSync(path.join(root, input))).digest('hex');
let status = 1;
if (!diagnostics.length) {
  const result = spawnSync(process.execPath, ['--test', path.join(here, 'Export.test.cjs')], { encoding: 'utf8', timeout: 30000, env: { ...process.env, JOBS_IM_EXPORT_WORK: work, JOBS_IM_EXPORT_SQLITE: path.join(here, 'SyncSqlite.py') } });
  const output = (result.stdout || '') + (result.stderr || ''); fs.writeFileSync(path.join(work, '原始结果.log'), output); process.stdout.write(output); status = result.status ?? 1;
} else process.stdout.write(fs.readFileSync(path.join(work, '类型检查.log'), 'utf8') + '\n');
fs.writeFileSync(path.join(work, '源SHA.json'), JSON.stringify({ native: false, network: false, sqlite: 'Python adapter, not Harmony RDB NAPI', fileSystem: 'Node adapter, not Harmony NAPI or a system document provider', hashes }, null, 2) + '\n');
fs.writeFileSync(path.join(work, '结果.json'), JSON.stringify({ exitCode: status, typeErrors: diagnostics.length, actualDevice: false, https: false, sourceCount: Object.keys(hashes).length }, null, 2) + '\n');
// Generated host modules are transient execution inputs; only logs and exact source hashes are evidence.
for (const name of fs.readdirSync(work)) if (/\.(js|ts)$/.test(name)) fs.unlinkSync(path.join(work, name));
process.stdout.write('Host report: ' + path.relative(root, work) + '\n'); process.exitCode = status;
