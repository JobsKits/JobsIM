// Created by Jobs on 2026年11月16日，星期一.
// Explicitly restart one managed isolated App to select a frozen Web directory.
const fs = require("node:fs"), path = require("node:path"), cp = require("node:child_process"), https = require("node:https"), crypto = require("node:crypto");
const args = process.argv.slice(2), options = {};
for (let i = 0; i < args.length; i++) {
  const key = args[i];
  if (["--allow-owned-app-restart", "--clients-closed"].includes(key)) options[key] = true;
  else if (["--environment-file", "--web-dir", "--expected-binary-sha"].includes(key) && args[i + 1] && !args[i + 1].startsWith("--")) options[key] = args[++i];
  else throw Error("Unsupported or incomplete argument: " + key);
}
if (!options["--allow-owned-app-restart"] || !options["--clients-closed"]) throw Error("Explicit owned restart and closed-client assertions are required");
if (!options["--environment-file"] || !options["--web-dir"] || !/^[0-9a-f]{64}$/.test(options["--expected-binary-sha"] || "")) throw Error("Explicit manifest, candidate directory and expected backend SHA required");
const manifest = path.resolve(options["--environment-file"]), before = fs.readFileSync(manifest), original = JSON.parse(before), webDir = fs.realpathSync(options["--web-dir"]);
const check = (condition, message) => { if (!condition) throw Error(message); };
const hash = file => crypto.createHash("sha256").update(fs.readFileSync(file)).digest("hex");
const sleep = ms => new Promise(resolve => setTimeout(resolve, ms));
const candidateHTML = fs.readFileSync(path.join(webDir, "index.html"), "utf8"), js = /assets\/(index-[^" ]+\.js)/.exec(candidateHTML)?.[1];
check(js && fs.statSync(path.join(webDir, "assets", js)).isFile(), "Frozen Web product is missing");
check(original.restoredDraftRealm && original.allowOwnedRestartForTests && original.serverURL === "https://127.0.0.1:54443", "Only the explicitly managed restored test realm is supported");
check(original.binarySHA256 === options["--expected-binary-sha"] && hash(original.binary) === original.binarySHA256 && original.serverCommand[0] === original.binary, "Frozen backend differs from the expected SHA");
const argument = (command, key) => { const value = command.find(v => v.startsWith(key + "=")); return value ? value.slice(key.length + 1) : command.indexOf(key) >= 0 ? command[command.indexOf(key) + 1] : undefined; };
check(argument(original.serverCommand, "--service") === "all" && argument(original.serverCommand, "--addr") === "127.0.0.1:54443" && argument(original.serverCommand, "--state-dir") === original.stateDir, "Managed role, address or state differs");
check(original.businessURL && original.journalURL && original.natsURL && fs.statSync(original.natsTokenFile).isFile(), "Complete original database and NATS environment required");
check(Number(fs.statfsSync(webDir).bavail) * Number(fs.statfsSync(webDir).bsize) >= 4 * 1024 ** 3, "4GiB operator disk gate");
const audit = path.join(path.dirname(manifest), "WebRuntime-" + Date.now() + "-" + crypto.randomUUID().slice(0, 8));
const agent = new https.Agent({ ca: fs.readFileSync(original.caPath) });
let stopped = false, candidate;
// Keep certificate verification enabled and limit each readiness request.
function get(route) {
  return new Promise((resolve, reject) => {
    const q = https.get(original.serverURL + route, { agent }, response => {
      const chunks = []; response.on("data", chunk => chunks.push(chunk));
      response.on("end", () => response.statusCode === 200 ? resolve(Buffer.concat(chunks).toString()) : reject(Error("HTTP_" + response.statusCode)));
    });
    q.setTimeout(2000, () => q.destroy(Error("bounded HTTPS timeout"))); q.on("error", reject);
  });
}
// Process absence and zombie state are terminal; no broad process matching.
function processState(pid) {
  const result = cp.spawnSync("/bin/ps", ["-p", String(pid), "-o", "stat="], { encoding: "utf8", timeout: 3000 });
  if (result.error) throw Error("Cannot verify managed process state");
  return result.stdout.trim();
}
// Signal only the byte-identical managed command and never force-kill.
async function stop(info) {
  const actual = cp.execFileSync("/bin/ps", ["-p", String(info.serverPID), "-o", "command="], { encoding: "utf8", timeout: 3000 }).trim();
  check(actual === info.serverCommand.join(" "), "PID command mismatch; no signal sent");
  process.kill(info.serverPID, "SIGTERM");
  const deadline = Date.now() + 10000;
  while (Date.now() < deadline) { const value = processState(info.serverPID); if (!value || value.startsWith("Z")) return; await sleep(50); }
  throw Error("Owned application did not stop within the bounded wait");
}
// Reuse the original identity and dependencies; no database or NATS stop occurs.
function launch(command, label) {
  const env = {};
  for (const [key, value] of Object.entries(process.env)) if (!key.startsWith("PG") && !key.startsWith("JOBS_IM_") && key !== "DATABASE_URL" && key !== "JOURNAL_DATABASE_URL" && !key.endsWith("_DATABASE_URL")) env[key] = value;
  Object.assign(env, { PGPASSFILE: original.pgPassFile, GOMAXPROCS: "2", JOBS_IM_DATABASE_URL: original.businessURL, JOBS_IM_JOURNAL_DATABASE_URL: original.journalURL, JOBS_IM_PUBLIC_URL: original.serverURL, JOBS_IM_NATS_URL: original.natsURL, JOBS_IM_NATS_TOKEN_FILE: original.natsTokenFile });
  const log = fs.openSync(path.join(audit, label + ".log"), "wx", 0o600);
  const child = cp.spawn(command[0], command.slice(1), { env, cwd: original.runDir, stdio: ["ignore", log, log], detached: true });
  child.unref(); fs.closeSync(log);
  return { ...original, serverPID: child.pid, serverCommand: command };
}
// Validate real instance and feature identity before publishing its new PID.
async function ready(info, expectedJS) {
  const deadline = Date.now() + 30000;
  while (Date.now() < deadline) {
    const value = processState(info.serverPID); check(value && !value.startsWith("Z"), "New managed App exited");
    let boot; try { boot = JSON.parse(await get("/api/bootstrap")); } catch { await sleep(150); continue; }
    check(boot.deploymentId === original.deploymentId && boot.restoreEpoch === original.restoreEpoch, "Instance identity or restore epoch changed");
    check(boot.features.sync_self_state === true && boot.features.cloud_drafts === true, "Original NATS-backed capabilities are unavailable");
    if (expectedJS) check((await get("/")).includes(expectedJS), "Wrong Web product served");
    return;
  }
  throw Error("Managed App readiness timed out");
}
// Publish atomically while retaining the exact original manifest in the audit.
function publish(info) {
  const temporary = manifest + ".web-" + process.pid, fd = fs.openSync(temporary, "wx", 0o600);
  fs.writeFileSync(fd, JSON.stringify(info, null, 2)); fs.fsyncSync(fd); fs.closeSync(fd); fs.renameSync(temporary, manifest);
  const parent = fs.openSync(path.dirname(manifest), "r"); fs.fsyncSync(parent); fs.closeSync(parent);
}
(async () => {
  console.log("Restarting the explicitly selected owned service=all test App; its ten listeners briefly stop. Original PG, NATS, identity and certificates are retained. Exact input is backed up privately; failure attempts the original command with its full environment.");
  await ready(original);
  const listener = cp.execFileSync("/usr/sbin/lsof", ["-nP", "-a", "-p", String(original.serverPID), "-iTCP:54443", "-sTCP:LISTEN", "-Fn"], { encoding: "utf8", timeout: 3000 });
  check(listener.includes("n127.0.0.1:54443"), "Listener PID differs");
  fs.mkdirSync(audit, { mode: 0o700 }); fs.writeFileSync(path.join(audit, "original-environment.json"), before, { flag: "wx", mode: 0o600 });
  check(fs.readFileSync(manifest).equals(before), "Manifest changed before stop");
  await stop(original); stopped = true;
  const command = [...original.serverCommand], index = command.indexOf("--web-dir"), inline = command.findIndex(value => value.startsWith("--web-dir="));
  if (index >= 0) command[index + 1] = webDir; else if (inline >= 0) command[inline] = "--web-dir=" + webDir; else command.push("--web-dir", webDir);
  candidate = launch(command, "candidate"); await ready(candidate, js);
  publish({ ...candidate, webDir, webRestartAudit: audit, status: "restored_draft_web_s2_candidate_ready" });
  const result = { status: "ready", serverPID: candidate.serverPID, deploymentId: original.deploymentId, restoreEpoch: original.restoreEpoch, backendSHA256: original.binarySHA256, webJS: js, webSHA256: hash(path.join(webDir, "assets", js)), strictSelectedCA: true, scope: "same owned service=all App; all ten listeners restarted", at: new Date().toISOString() };
  fs.writeFileSync(path.join(audit, "public-summary.json"), JSON.stringify(result, null, 2), { mode: 0o600, flag: "wx" }); console.log(JSON.stringify(result));
})().catch(async error => {
  let recovered = false;
  try {
    if (stopped) {
      const state = candidate && processState(candidate.serverPID); if (state && !state.startsWith("Z")) await stop(candidate);
      const old = launch(original.serverCommand, "original-recovery"); await ready(old); publish(old); recovered = true;
    }
  } catch { /* Preserve the original error and private recovery log. */ }
  const result = { status: "failed", reason: error.message, originalWithFullEnvironmentRestored: recovered, at: new Date().toISOString() };
  if (fs.existsSync(audit)) fs.writeFileSync(path.join(audit, "failure-summary.json"), JSON.stringify(result, null, 2), { mode: 0o600, flag: "wx" });
  console.error(JSON.stringify(result)); process.exitCode = 1;
}).finally(() => agent.destroy());
