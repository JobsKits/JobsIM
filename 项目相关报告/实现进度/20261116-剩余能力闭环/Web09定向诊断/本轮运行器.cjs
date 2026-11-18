// Created by Jobs on 2026年11月17日，星期二.
// Bounded fixture diagnostics; never contacts a real JobsIM instance.
const fs = require('node:fs'), path = require('node:path'), os = require('node:os'), crypto = require('node:crypto');
const { spawn } = require('node:child_process'), { pathToFileURL } = require('node:url');
const root = process.cwd(), web = path.join(root, '代码/前端/Web'), base = __dirname;
const driver = path.join(root, '测试/前端/Web/auth-session.cjs/auth-session.cjs');
const free = () => { const s = fs.statfsSync(root); return Number(s.bavail) * Number(s.bsize); };
const sha = file => crypto.createHash('sha256').update(fs.readFileSync(file)).digest('hex');
const files = fs.readdirSync(path.join(web, 'Src')).filter(n => /\.(ts|tsx|css)$/.test(n)).map(n => path.join(web, 'Src', n));
for (const n of ['index.html', 'package.json', 'package-lock.json', 'tsconfig.json', 'vite.config.ts']) files.push(path.join(web, n));
const walk = directory => { for (const e of fs.readdirSync(directory, { withFileTypes: true })) { const p = path.join(directory, e.name); e.isDirectory() ? walk(p) : e.isFile() && files.push(p); } };
walk(path.join(web, 'Public')); files.push(driver, __filename);
const snapshot = () => Object.fromEntries(files.map(p => [path.relative(root, p), sha(p)]));
const initial = snapshot(), stamp = Date.now(), results = [];
if (free() < 4 * 1024 ** 3) throw Error('DISK_GATE');
fs.writeFileSync(path.join(base, '诊断输入SHA256.json'), JSON.stringify(initial, null, 2) + '\n', { flag: 'wx' });
for (const p of files) { const q = path.join(base, '诊断输入源码', path.relative(root, p)); fs.mkdirSync(path.dirname(q), { recursive: true }); fs.copyFileSync(p, q, fs.constants.COPYFILE_EXCL); }
const variants = [
  { trailingSlash: true, concurrentImports: false }, { trailingSlash: false, concurrentImports: false },
  { trailingSlash: true, concurrentImports: true }, { trailingSlash: false, concurrentImports: true },
  { trailingSlash: true, concurrentImports: false }, { trailingSlash: false, concurrentImports: false },
  { trailingSlash: true, concurrentImports: true }, { trailingSlash: false, concurrentImports: true },
  { trailingSlash: true, concurrentImports: false }, { trailingSlash: true, concurrentImports: true },
];
const same = value => JSON.stringify(value) === JSON.stringify(initial);
(async () => {
  const { createServer } = await import(pathToFileURL(path.join(web, 'node_modules/vite/dist/node/index.js')).href);
  for (let index = 0; index < variants.length; index++) {
    const variant = variants[index], folder = path.join(base, '第' + String(index + 1).padStart(2, '0') + '轮');
    if (!same(snapshot())) throw Error('SOURCE_DRIFT_BEFORE_RUN');
    fs.mkdirSync(folder, { recursive: false });
    const cache = path.join(os.homedir(), 'Library/Application Support/JobsIM/Build/Web/AuthDiagnostic', String(stamp), String(index + 1), 'Vite');
    const events = [];
    const vite = await createServer({ root: web, configFile: false, publicDir: 'Public', cacheDir: cache, logLevel: 'silent', server: { host: '127.0.0.1', port: 0 } });
    let child, reason, code = -1, minimumFreeBytes = free(), timer, graph;
    const before = snapshot(), started = Date.now(), log = fs.openSync(path.join(folder, '原始输出.log'), 'wx');
    try {
      vite.watcher.on('change', file => events.push({ at: Date.now(), file: path.relative(root, file) }));
      await vite.listen();
      const url = new URL(vite.resolvedUrls.local[0]);
      const address = url.origin + (variant.trailingSlash ? '/' : '');
      const mode = (variant.trailingSlash ? 'base-with-trailing-slash' : 'base-no-trailing-slash') + '+' + (variant.concurrentImports ? 'parallel-cold-module-imports' : 'sequential-cold-module-imports');
      child = spawn(process.execPath, [driver], { cwd: root, env: { ...process.env, JOBSIM_WEB_URL: address, JOBSIM_AUTH_REPORT: path.join(folder, '浏览器'), JOBSIM_AUTH_VARIANT: mode, JOBSIM_AUTH_CONCURRENT_COLD_IMPORTS: variant.concurrentImports ? '1' : '0' }, detached: true, stdio: ['ignore', log, log] });
      timer = setInterval(() => {
        minimumFreeBytes = Math.min(minimumFreeBytes, free());
        if (minimumFreeBytes < 4 * 1024 ** 3 || Date.now() - started > 180000) {
          reason = minimumFreeBytes < 4 * 1024 ** 3 ? 'DISK_GATE' : 'TIMEOUT';
          try { process.kill(-child.pid, 'SIGTERM'); } catch (_) {}
        }
      }, 200);
      code = await new Promise((resolve, reject) => { child.on('error', reject); child.on('exit', value => resolve(value ?? -1)); });
      try {
        const modules = vite.environments?.client?.moduleGraph?.urlToModuleMap ?? vite.moduleGraph?.urlToModuleMap;
        graph = modules ? [...modules].filter(([url]) => /AuthSession\.ts|\/Src\/api\.ts/.test(url)).map(([url, item]) => ({ url, id: item.id, importers: [...item.importers].map(entry => entry.url) })) : 'unavailable';
      } catch (error) { graph = { unavailable: error.name }; }
      const after = snapshot();
      const resultFile = path.join(folder, '浏览器', code === 0 ? '浏览器协议结果.json' : '认证失败诊断.json');
      const result = fs.existsSync(resultFile) ? JSON.parse(fs.readFileSync(resultFile)) : undefined;
      const metadata = { index: index + 1, variant, mode, url: address, freshViteCache: cache, freshBrowserProcessAndContext: true,
        startedAt: new Date(started).toISOString(), elapsedSeconds: (Date.now() - started) / 1000, exitCode: code, reason,
        minimumFreeBytes, before, after, inputsUnchanged: same(before) && same(after), completedGroups: result?.checks?.length ?? 0,
        result: fs.existsSync(resultFile) ? path.relative(root, resultFile) : null, serverModuleGraph: graph, fileEvents: events };
      fs.writeFileSync(path.join(folder, '执行.json'), JSON.stringify(metadata, null, 2) + '\n');
      results.push(metadata);
      console.log(JSON.stringify({ round: index + 1, exitCode: code, groups: metadata.completedGroups, mode, inputsUnchanged: metadata.inputsUnchanged }));
      if (code !== 0 || !metadata.inputsUnchanged || reason) break;
    } finally {
      clearInterval(timer); fs.closeSync(log); await vite.close();
    }
  }
  const final = { startedAt: new Date(stamp).toISOString(), finishedAt: new Date().toISOString(), maximumRuns: 10, stoppedOnFirstFailure: results.some(r => r.exitCode !== 0),
    actualRuns: results.length, failedRuns: results.filter(r => r.exitCode !== 0).map(r => r.index),
    productionUnchanged: same(snapshot()), originalTestAssertionsRetained: true, realBackend: false,
    interpretation: 'Bounded diagnostics only. Passing attempts do not explain or repair the original Web09 failure.',
    rounds: results.map(({ before, after, serverModuleGraph, fileEvents, ...summary }) => summary) };
  fs.writeFileSync(path.join(base, '本轮结论.json'), JSON.stringify(final, null, 2) + '\n');
  console.log(JSON.stringify({ actualRuns: final.actualRuns, failedRuns: final.failedRuns, productionUnchanged: final.productionUnchanged }));
  if (!final.productionUnchanged) process.exitCode = 1;
})().catch(error => { console.error(error.name + ': ' + error.message); process.exitCode = 1; });
