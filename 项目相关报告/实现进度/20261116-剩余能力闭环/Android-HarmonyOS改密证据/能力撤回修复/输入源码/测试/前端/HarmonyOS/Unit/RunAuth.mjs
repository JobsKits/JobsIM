import fs from 'node:fs';
import path from 'node:path';
import os from 'node:os';
import crypto from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath } from 'node:url';
import { spawnSync } from 'node:child_process';

const ownDirectory = path.dirname(fileURLToPath(import.meta.url));
const repository = path.resolve(ownDirectory, '../../../..');
const clt = process.env.JOBS_IM_HARMONY_CLT_HOME || path.join(os.homedir(), 'Library/Application Support/JobsIM/Toolchains/HarmonyMirror/ArkTS-6.1.1.280/command-line-tools');
const require = createRequire(import.meta.url);
const ts = require(path.join(clt, 'sdk/default/openharmony/ets/build-tools/ets-loader/node_modules/typescript'));
const workRoot = path.join(os.homedir(), 'Library/Application Support/JobsIM/Build/Harmony');
fs.mkdirSync(workRoot, { recursive: true });
const work = fs.mkdtempSync(path.join(workRoot, 'AuthRules-'));
const data = path.join(repository, '代码/前端/HarmonyOS/entry/src/main/ets/Data');
const hashes = {};
for (const name of ['Models', 'Strings', 'AuthCoordinator', 'SyncProtocol', 'SelfProtocol', 'ExportProtocol', 'Api']) {
  const file = path.join(data, name + '.ets');
  const source = fs.readFileSync(file, 'utf8');
  const result = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS }, reportDiagnostics: true, fileName: name + '.ts' });
  if ((result.diagnostics || []).some(item => item.category === ts.DiagnosticCategory.Error)) {
    throw new Error(name + ' syntax check failed');
  }
  fs.writeFileSync(path.join(work, name + '.js'), result.outputText);
  fs.writeFileSync(path.join(work, name + '.ts'), source);
  hashes[path.relative(repository, file)] = crypto.createHash('sha256').update(source).digest('hex');
}
for (const relative of ['代码/前端/HarmonyOS/entry/src/main/ets/Data/Store.ets',
  '代码/前端/HarmonyOS/entry/src/main/ets/Data/NativeServices.ets',
  '代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets',
  '代码/前端/HarmonyOS/entry/src/main/ets/entryability/EntryAbility.ets',
  '测试/前端/HarmonyOS/OhosTest/ets/test/List.test.ets',
  ...['RunAuth.mjs', 'Policies.test.mjs', 'Auth.test.cjs', 'AuthApi.test.cjs'].map(file => '测试/前端/HarmonyOS/Unit/' + file)]) {
  hashes[relative] = crypto.createHash('sha256').update(fs.readFileSync(path.join(repository, relative))).digest('hex');
}
const program = ts.createProgram(['Models.ts', 'AuthCoordinator.ts'].map(file => path.join(work, file)), {
  target: ts.ScriptTarget.ES2020, module: ts.ModuleKind.CommonJS, strict: true, skipLibCheck: true, noEmit: true
});
const diagnostics = ts.getPreEmitDiagnostics(program);
if (diagnostics.length > 0) {
  for (const item of diagnostics) process.stderr.write(ts.flattenDiagnosticMessageText(item.messageText, ' ') + '\n');
  process.exit(1);
}
const files = ['Policies.test.mjs', 'Auth.test.cjs', 'AuthApi.test.cjs'].map(file => path.join(ownDirectory, file)).filter(file => fs.existsSync(file));
const result = spawnSync(process.execPath, ['--test', ...files], {
  encoding: 'utf8', timeout: 30000,
  env: { ...process.env, JOBS_IM_HARMONY_HOST_MODELS: path.join(work, 'Models.js'), JOBS_IM_HARMONY_HOST_STRINGS: path.join(work, 'Strings.js'),
    JOBS_IM_HARMONY_HOST_AUTH: path.join(work, 'AuthCoordinator.js'), JOBS_IM_HARMONY_HOST_API: path.join(work, 'Api.js'),
    JOBS_IM_HARMONY_UI_SOURCE: path.join(repository, '代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets') }
});
const output = (result.stdout || '') + (result.stderr || '');
fs.writeFileSync(path.join(work, 'Results.txt'), output);
fs.writeFileSync(path.join(work, 'SourceManifest.json'), JSON.stringify({ node: process.version, work, hashes, native: false }, null, 2) + '\n');
process.stdout.write(output);
process.stdout.write('Host-only work: ' + work + '\n');
process.exitCode = result.status === null ? 1 : result.status;
