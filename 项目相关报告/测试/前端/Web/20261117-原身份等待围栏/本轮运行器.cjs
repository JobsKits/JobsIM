// Created by Jobs on 2026年11月17日，星期二.
// One-shot local runner: no dependency install, real API or service restart.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), { spawn } = require("node:child_process"), { pathToFileURL } = require("node:url");
const root = path.resolve(__dirname, "../../../../.."), web = path.join(root, "代码/前端/Web"), base = __dirname;
const free = () => { const value = fs.statfsSync(root); return Number(value.bavail) * Number(value.bsize); };
const gate = () => { if (free() < 4 * 1024 ** 3) throw Error("4GiB disk gate"); };
const walk = dir => fs.readdirSync(path.join(root, dir), { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? walk(path.join(dir, entry.name)) : [path.join(dir, entry.name)]);
const sources = [...walk("代码/前端/Web/Src"), ...walk("代码/前端/Web/Public"), ...["index.html", "package.json", "package-lock.json", "tsconfig.json", "vite.config.ts"].map(name => "代码/前端/Web/" + name), ...["auth-session", "password-change", "account-deletion", "data-export", "group-governance"].map(name => "测试/前端/Web/" + name + ".cjs/" + name + ".cjs")];
const hashes = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const save = (file, value) => fs.writeFileSync(file, JSON.stringify(value, null, 2) + "\n");
async function run(name, args, extraEnv = {}) {
  gate(); const dir = path.join(base, name); if (fs.existsSync(dir)) throw Error("Refusing existing stage " + name); fs.mkdirSync(dir);
  const before = hashes(), startedAt = new Date().toISOString(), freeBefore = free(); save(path.join(dir, "输入SHA-执行前.json"), before);
  const log = fs.openSync(path.join(dir, "进程.log"), "wx"), child = spawn(process.execPath, args, { cwd: root, detached: true, env: { ...process.env, ...extraEnv }, stdio: ["ignore", log, log] });
  let stopped = "", elapsed = 0, minimumFree = freeBefore;
  const stop = why => { if (stopped) return; stopped = why; try { process.kill(-child.pid, "SIGTERM"); } catch {} setTimeout(() => { try { process.kill(-child.pid, "SIGKILL"); } catch {} }, 2000).unref(); };
  const timer = setInterval(() => { elapsed++; minimumFree = Math.min(minimumFree, free()); if (minimumFree < 4 * 1024 ** 3) stop("4GiB disk watchdog"); else if (elapsed >= 180) stop("180s watchdog"); }, 1000);
  const code = await new Promise((resolve, reject) => { child.on("error", reject); child.on("exit", (code, signal) => resolve({ code, signal })); }).finally(() => { clearInterval(timer); fs.closeSync(log); });
  const after = hashes(); save(path.join(dir, "输入SHA-执行后.json"), after);
  const result = { startedAt, finishedAt: new Date().toISOString(), command: [process.execPath, ...args], exit: code, stopped, freeBefore, minimumFree, freeAfter: free(), inputsUnchanged: JSON.stringify(before) === JSON.stringify(after), actualAPI: false, dependencyInstall: false };
  save(path.join(dir, "进程.json"), result); console.log(name, JSON.stringify(result));
  if (code.code !== 0 || stopped || !result.inputsUnchanged) throw Error("Stage failed: " + name);
}
(async () => {
  gate();
  if (process.argv.includes("--build")) {
    const { build } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href);
    await build({ root: web, configFile: false, publicDir: "Public", cacheDir: process.env.JOBSIM_PRIVATE_CACHE, build: { outDir: process.env.JOBSIM_PRIVATE_BUILD, emptyOutDir: false, sourcemap: true } }); return;
  }
  const closure = hashes(); save(path.join(base, "本轮源码闭包SHA.json"), closure);
  for (const file of sources) { const dest = path.join(base, "本轮输入源码", file); fs.mkdirSync(path.dirname(dest), { recursive: true }); fs.copyFileSync(path.join(root, file), dest); }
  await run("06-类型检查", [path.join(web, "node_modules/typescript/bin/tsc"), "--noEmit", "-p", path.join(web, "tsconfig.json")]);
  const { createServer } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href), work = fs.mkdtempSync(path.join(os.tmpdir(), "JobsIM-AuthFence-"));
  const vite = await createServer({ root: web, configFile: false, publicDir: "Public", cacheDir: path.join(work, "Vite"), server: { host: "127.0.0.1", port: 0 } });
  try { await vite.listen(); await run("07-认证协议", [path.join(root, "测试/前端/Web/auth-session.cjs/auth-session.cjs")], { JOBSIM_WEB_URL: vite.resolvedUrls.local[0].replace(/\/$/, ""), JOBSIM_AUTH_REPORT: path.join(base, "07-认证协议", "浏览器") }); } finally { await vite.close(); }
  for (const [stage, name, key] of [["08-改密", "password-change", "JOBSIM_PASSWORD_REPORT"], ["09-注销", "account-deletion", "JOBSIM_DELETION_REPORT"], ["10-本人导出", "data-export", "JOBSIM_EXPORT_REPORT"]]) {
    await run(stage, [path.join(root, "测试/前端/Web/" + name + ".cjs/" + name + ".cjs")], { [key]: path.join(base, stage, "浏览器") });
  }
  const buildDir = path.join(os.homedir(), "Library/Application Support/JobsIM/Build/Web/20261117-AuthFence-" + process.pid);
  if (fs.existsSync(buildDir)) throw Error("Private build directory exists");
  await run("11-私有生产构建", [__filename, "--build"], { JOBSIM_PRIVATE_BUILD: buildDir, JOBSIM_PRIVATE_CACHE: path.join(work, "BuildCache") });
  const buildHashes = dir => fs.readdirSync(dir, { withFileTypes: true }).flatMap(entry => entry.isDirectory() ? buildHashes(path.join(dir, entry.name)) : [{ file: path.relative(buildDir, path.join(dir, entry.name)), sha256: crypto.createHash("sha256").update(fs.readFileSync(path.join(dir, entry.name))).digest("hex"), bytes: fs.statSync(path.join(dir, entry.name)).size }]);
  save(path.join(base, "11-私有生产构建", "产物SHA.json"), { buildDir, files: buildHashes(buildDir), sourceUnchanged: JSON.stringify(closure) === JSON.stringify(hashes()) });
})().catch(error => { console.error(error); process.exitCode = 1; });
