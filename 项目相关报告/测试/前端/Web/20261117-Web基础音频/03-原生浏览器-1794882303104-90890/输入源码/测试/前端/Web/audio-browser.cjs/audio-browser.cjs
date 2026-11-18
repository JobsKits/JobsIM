// Created by Jobs on 2026年11月17日，星期二.
// Native browser media + synthetic audio track only. API and identities are explicit fixtures.
const fs = require("node:fs"), path = require("node:path"), os = require("node:os"), crypto = require("node:crypto"), assert = require("node:assert/strict"), { pathToFileURL } = require("node:url");
const { chromium } = require(process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright");
const root = path.resolve(__dirname, "../../../.."), web = path.join(root, "代码/前端/Web"), out = process.env.JOBSIM_AUDIO_BROWSER_REPORT || path.join(root, "项目相关报告/测试/前端/Web/20261117-Web基础音频", "浏览器-" + Date.now());
const free = () => { const s = fs.statfsSync(root); return Number(s.bavail) * Number(s.bsize); };
if (free() < 4 * 1024 ** 3) throw Error("4GiB disk gate");
if (fs.existsSync(out)) throw Error("Choose a new report directory"); fs.mkdirSync(out, { recursive: true });
const sources = fs.readdirSync(path.join(web, "Src")).filter(name => /\.(ts|tsx|css)$/.test(name)).map(name => path.join("代码/前端/Web/Src", name)).concat(path.relative(root, __filename));
const hashes = () => Object.fromEntries(sources.map(file => [file, crypto.createHash("sha256").update(fs.readFileSync(path.join(root, file))).digest("hex")]));
const before = hashes(), checks = [], startedAt = new Date().toISOString();
(async () => {
  const { createServer } = await import(pathToFileURL(path.join(web, "node_modules/vite/dist/node/index.js")).href), work = fs.mkdtempSync(path.join(os.tmpdir(), "JobsIM-Audio-"));
  const vite = await createServer({ root: web, configFile: false, publicDir: "Public", cacheDir: path.join(work, "Vite"), server: { host: "127.0.0.1", port: 0 } });
  let browser;
  try {
    await vite.listen(); browser = await chromium.launch({ headless: true, executablePath: process.env.BROWSER_EXECUTABLE || "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", args: ["--autoplay-policy=no-user-gesture-required", "--mute-audio"] });
    const context = await browser.newContext(), page = await context.newPage();
    await page.route("**/api/**", route => route.abort());
    await page.goto(vite.resolvedUrls.local[0], { waitUntil: "domcontentloaded" });
    await page.evaluate(async () => {
      const panel = await import("/Src/AudioRecorderPanel.tsx"), media = await import("/Src/PrivateMedia.tsx"), auth = await import("/Src/AuthSession.ts"), store = await import("/Src/PlaybackCheckpointStore.ts"), rules = await import("/Src/PlaybackCheckpointProtocol.ts"), globals = await import("/Src/GlobalMessages.ts"), database = await import("/Src/StorageDatabase.ts"), i18n = await import("/Src/i18n.ts");
      const resources = performance.getEntriesByType("resource").map(row => row.name), reactURL = resources.find(url => /\/react\.js\?/.test(url)), domURL = resources.find(url => /\/react-dom_client\.js\?/.test(url));
      const R = await import(reactURL), D = await import(domURL), React = R.default ?? R, dom = D.default ?? D, db = await database.storageDatabase;
      const write = values => new Promise((resolve, reject) => { const tx = db.transaction("spaces", "readwrite", { durability: "strict" }); for (const [key, value] of values) tx.objectStore("spaces").put(value, key); tx.oncomplete = resolve; tx.onerror = () => reject(tx.error); });
      const read = key => new Promise((resolve, reject) => { const tx = db.transaction("spaces"), q = tx.objectStore("spaces").get(key); tx.oncomplete = () => resolve(q.result); tx.onerror = () => reject(tx.error); });
      const f = { current: true, microphones: 0, mode: "normal", sendMode: "fail", files: [], tracks: [], locale: "en", owner: "", log: [], unavailable: 0 };
      f.ids = { deployment: crypto.randomUUID(), user: crypto.randomUUID(), device: crypto.randomUUID(), family: crypto.randomUUID(), conversation: crypto.randomUUID(), message: crypto.randomUUID(), attachment: crypto.randomUUID(), client: crypto.randomUUID() };
      f.namespace = [location.origin, f.ids.deployment, f.ids.user, "personal", "3", f.ids.device].join("|"); f.owner = f.namespace + "|" + f.ids.family;
      f.install = family => { auth.configureAuthProtocol(true, f.ids.deployment, true); auth.rememberPublicAuth({ deploymentId: f.ids.deployment, userId: f.ids.user, username: "audio_fixture", deviceId: f.ids.device, familyId: family ?? f.ids.family, sourceContextId: "personal", restoreEpoch: "3", refreshGeneration: "0", accessExpiresAt: new Date(Date.now() + 600000).toISOString(), idleExpiresAt: new Date(Date.now() + 1200000).toISOString(), absoluteExpiresAt: new Date(Date.now() + 1800000).toISOString() }); };
      f.install(); await write([["auth-active|" + rules.playbackScope(f.namespace), f.namespace]]);
      f.synthetic = async () => {
        const audio = new AudioContext(), oscillator = audio.createOscillator(), gain = audio.createGain(), target = audio.createMediaStreamDestination(); gain.gain.value = 0.01; oscillator.frequency.value = 220; oscillator.connect(gain); gain.connect(target); oscillator.start(); await audio.resume();
        const track = target.stream.getAudioTracks()[0]; f.tracks.push(track); f.audioContexts ??= []; f.audioContexts.push({ audio, oscillator }); return target.stream;
      };
      navigator.mediaDevices.getUserMedia = async () => { f.microphones++; if (f.mode === "deny") throw new DOMException("Fixture denial", "NotAllowedError"); const stream = await f.synthetic(); if (f.mode === "late") return await new Promise(resolve => { f.releasePermission = () => resolve(stream); }); return stream; };
      const nativeFetch = window.fetch.bind(window); f.nativeFetch = nativeFetch;
      f.message = () => ({ id: f.ids.message, conversationId: f.ids.conversation, senderId: f.ids.user, clientMsgId: f.ids.client, type: "audio", text: "synthetic.wav", attachmentId: f.ids.attachment, createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), version: "1", contentVersion: "1", sequence: "1", deleted: false, reactions: [], pinned: false, favorite: false });
      const wav = new Uint8Array(44 + 8000 * 2 * 8), view = new DataView(wav.buffer), text = (offset, value) => { for (let i = 0; i < value.length; i++) wav[offset + i] = value.charCodeAt(i); };
      text(0, "RIFF"); view.setUint32(4, wav.length - 8, true); text(8, "WAVEfmt "); view.setUint32(16, 16, true); view.setUint16(20, 1, true); view.setUint16(22, 1, true); view.setUint32(24, 8000, true); view.setUint32(28, 16000, true); view.setUint16(32, 2, true); view.setUint16(34, 16, true); text(36, "data"); view.setUint32(40, wav.length - 44, true); f.wav = new Blob([wav], { type: "audio/wav" });
      window.fetch = async (url, options = {}) => {
        if (String(url).startsWith("blob:")) return nativeFetch(url, options);
        const route = new URL(url, location.origin).pathname; f.log.push({ route, method: options.method ?? "GET", family: new Headers(options.headers).get("X-JobsIM-Auth-Family") });
        if (route === "/api/bootstrap") return Response.json({ deploymentId: f.ids.deployment, restoreEpoch: "3", features: { auth_refresh: true, auth_retirement_chain: true, media: true } });
        if (route === "/api/auth/csrf") return Response.json({ csrfToken: "fixture-csrf" });
        if (route === "/api/messages/" + f.ids.message) return f.denyPlayback ? Response.json({ error: { code: "NOT_A_MEMBER" } }, { status: 403 }) : Response.json(f.message());
        if (route === "/api/media/" + f.ids.attachment) return new Response(f.wav, { headers: { "Content-Type": "audio/wav" } });
        return Response.json({ error: { code: "FIXTURE_ROUTE_BLOCKED" } }, { status: 503 });
      };
      const container = document.createElement("div"); container.id = "audio-fixture"; container.className = "settings-content"; container.style.cssText = "width:min(640px,calc(100vw - 32px));margin:20px auto"; document.body.append(container); document.getElementById("root").style.display = "none"; const ui = dom.createRoot(container);
      f.renderRecorder = () => ui.render(React.createElement(panel.default, { key: f.owner, contextKey: f.owner, disabled: false, locale: f.locale, onSend: async (file, current) => { f.files.push(file); if (f.sendMode === "hold") await new Promise(resolve => { f.releaseSend = resolve; }); return current() && f.sendMode === "ok"; } }));
      f.renderMedia = key => ui.render(React.createElement(media.default, { key, path: "/api/media/" + f.ids.attachment, type: "audio", name: "synthetic.wav", version: "1", t: key => i18n.translate(f.locale, key), playback: { namespace: f.namespace, messageId: f.ids.message, conversationId: f.ids.conversation, contentVersion: "1", familyId: f.ids.family, locale: f.locale } }));
      f.empty = () => ui.render(null); f.write = write; f.read = read; f.auth = auth; f.store = store; f.rules = rules; f.globals = globals; window.audioTest = f; f.renderRecorder();
    });
    const fixture = page.locator("#audio-fixture"), test = async (name, work) => { assert.ok(await work(), name); checks.push(name); };
    await fixture.getByRole("button", { name: "Record voice", exact: true }).waitFor();
    await test("real recorder panel does not prompt before explicit start", () => page.evaluate(() => audioTest.microphones === 0));
    await fixture.getByRole("button", { name: "Record voice", exact: true }).click(); await fixture.getByRole("button", { name: "Stop and preview", exact: true }).waitFor(); await page.waitForTimeout(1200); await fixture.getByRole("button", { name: "Stop and preview", exact: true }).click(); await fixture.locator("audio").waitFor();
    const recorded = await page.evaluate(async () => { const blob = await (await audioTest.nativeFetch(document.querySelector("#audio-fixture audio").src)).blob(), bytes = new Uint8Array(await blob.arrayBuffer()); let binary = ""; for (const byte of bytes) binary += String.fromCharCode(byte); return { type: blob.type, bytes: bytes.length, prefix: [...bytes.slice(0, 4)], base64: btoa(binary), tracksStopped: audioTest.tracks.every(track => track.readyState === "ended"), sends: audioTest.files.length }; });
    fs.writeFileSync(path.join(out, recorded.type.includes("mp4") ? "原生合成音轨.m4a" : "原生合成音轨.webm"), Buffer.from(recorded.base64, "base64")); delete recorded.base64;
    await fixture.screenshot({ path: path.join(out, "录音试听-英文-白天.png") });
    await test("native MediaRecorder synthetic track stops into preview without sending or a live microphone", async () => recorded.tracksStopped && recorded.sends === 0 && recorded.bytes > 100 && ["audio/webm", "audio/mp4"].some(type => recorded.type.startsWith(type)));
    await fixture.getByRole("button", { name: "Send this recording", exact: true }).click();
    await fixture.getByRole("alert").waitFor();
    await test("failed upload keeps a real playable RAM preview for explicit retry", () => page.evaluate(() => audioTest.files.length === 1 && !!document.querySelector("#audio-fixture audio")));
    await page.evaluate(() => { audioTest.sendMode = "ok"; }); await fixture.getByRole("button", { name: "Send this recording", exact: true }).first().click();
    await fixture.getByRole("button", { name: "Record voice", exact: true }).waitFor();
    await test("successful explicit queue handoff clears preview and retry reuses the same recorded bytes", () => page.evaluate(async () => audioTest.files.length === 2 && audioTest.files[0].name === audioTest.files[1].name && audioTest.files[0].size === audioTest.files[1].size && !document.querySelector("#audio-fixture audio")));
    await page.evaluate(() => { audioTest.mode = "late"; }); await fixture.getByRole("button", { name: "Record voice", exact: true }).click(); await fixture.getByRole("button", { name: "Cancel recording", exact: true }).click(); await page.waitForFunction(() => !!audioTest.releasePermission); await page.evaluate(() => audioTest.releasePermission());
    await test("cancelling pending native permission closes the late synthetic stream", async () => { await page.waitForFunction(() => audioTest.tracks.every(track => track.readyState === "ended")); return true; });
    await page.evaluate(() => { audioTest.mode = "deny"; }); await fixture.getByRole("button", { name: "Record voice", exact: true }).click(); await fixture.getByRole("alert").waitFor();
    await test("microphone refusal has visible failure and an explicit retry", async () => (await fixture.getByRole("button", { name: "Record again", exact: true }).count()) === 1);
    await page.evaluate(() => { audioTest.mode = "normal"; }); await fixture.getByRole("button", { name: "Record again", exact: true }).click(); await fixture.getByRole("button", { name: "Stop and preview", exact: true }).waitFor();
    await page.evaluate(() => { Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "hidden" }); document.dispatchEvent(new Event("visibilitychange")); });
    await test("hidden document interrupts recording and stops native tracks without a preview", () => page.evaluate(() => audioTest.tracks.every(track => track.readyState === "ended") && !document.querySelector("#audio-fixture audio")));
    await page.evaluate(() => { Object.defineProperty(document, "visibilityState", { configurable: true, get: () => "visible" }); document.dispatchEvent(new Event("visibilitychange")); audioTest.renderMedia("playback-1"); });
    await fixture.locator("audio").waitFor(); await page.waitForFunction(() => Number.isFinite(document.querySelector("#audio-fixture audio")?.duration));
    await fixture.locator("audio").evaluate(element => element.play());
    await page.waitForFunction(() => { const element = document.querySelector("#audio-fixture audio"); return element && !element.paused && element.currentTime > 0.2; });
    await test("native play pause authorization order reaches real playback after exact message recheck", () => page.evaluate(() => audioTest.log.some(row => row.route === "/api/messages/" + audioTest.ids.message) && document.querySelector("#audio-fixture").textContent.includes("Played")));
    await fixture.locator("select").selectOption("1.5"); await test("speed control updates native playback rate", () => fixture.locator("audio").evaluate(element => element.playbackRate === 1.5));
    await fixture.locator("audio").evaluate(element => { element.currentTime = 2; element.pause(); });
    await page.waitForFunction(async () => (await audioTest.read("playback-checkpoints|" + audioTest.namespace))?.entries?.some(entry => entry.position > 0 && entry.heard));
    await test("actual strict IndexedDB persists only bound checkpoint metadata", () => page.evaluate(async () => { const row = await audioTest.read("playback-checkpoints|" + audioTest.namespace); return row.entries.length === 1 && row.entries[0].messageId === audioTest.ids.message && row.entries[0].contentVersion === "1" && /^[0-9a-f]{64}$/.test(row.entries[0].mediaDigest) && !/url|text|password|token/i.test(Object.keys(row.entries[0]).join(" ")); }));
    await page.evaluate(() => audioTest.renderMedia("playback-2")); await fixture.getByRole("button", { name: /Continue playback/ }).waitFor();
    await test("remount restores legal local resume position without autoplay", () => fixture.locator("audio").evaluate(element => element.paused && element.currentTime > 0));
    await page.evaluate(() => { audioTest.denyPlayback = true; }); await fixture.getByRole("button", { name: /Continue playback/ }).click(); await fixture.locator(".private-media-placeholder").waitFor();
    await page.waitForFunction(async () => !(await audioTest.read("playback-checkpoints|" + audioTest.namespace)).entries.length);
    await test("permission rejection stops playback and removes the exact checkpoint", () => page.evaluate(async () => !(await audioTest.read("playback-checkpoints|" + audioTest.namespace)).entries.length && !document.querySelector("#audio-fixture audio")));
    await page.evaluate(() => { audioTest.denyPlayback = false; audioTest.renderMedia("playback-3"); }); await fixture.locator("audio").waitFor(); await page.waitForFunction(() => Number.isFinite(document.querySelector("#audio-fixture audio")?.duration)); await fixture.locator("audio").evaluate(element => element.play()); await page.waitForFunction(() => !document.querySelector("#audio-fixture audio").paused);
    const oldURL = await fixture.locator("audio").getAttribute("src"); await page.evaluate(() => audioTest.install(crypto.randomUUID()));
    await fixture.locator(".private-media-placeholder").waitFor();
    await test("auth changes during continuous playback pause and revoke the old Blob URL before App rerender", () => page.evaluate(async url => { try { await audioTest.nativeFetch(url); return false; } catch { return !document.querySelector("#audio-fixture audio"); } }, oldURL));
    await page.evaluate(() => { audioTest.empty(); audioTest.install(); });
    await test("global strict transaction clears unmounted namespace and archive checkpoints but preserves another scope", () => page.evaluate(async () => {
      const f = audioTest, key = "playback-checkpoints|" + f.namespace, archive = key + "|archive|3", other = "playback-checkpoints|" + f.namespace.replace(f.ids.user, crypto.randomUUID()), unrelated = crypto.randomUUID();
      const entry = { messageId: f.ids.message, conversationId: f.ids.conversation, contentVersion: "1", mediaDigest: "a".repeat(64), kind: "audio", position: 1, duration: 8, heard: true, updatedAt: Date.now() }, holder = { schema: "1", entries: [entry, { ...entry, messageId: unrelated }] };
      await f.write([[key, holder], [archive, holder], [other, holder]]); await f.globals.observeVerifiedGlobals(f.namespace, [f.ids.message], () => true);
      const values = await Promise.all([f.read(key), f.read(archive), f.read(other)]); return values[0].entries.length === 1 && values[1].entries.length === 1 && values[2].entries.length === 2 && values[0].entries[0].messageId === unrelated;
    }));
    await test("global ledger blocks a late checkpoint save from resurrecting deleted metadata", () => page.evaluate(async () => { const f = audioTest, binding = { namespace: f.namespace, messageId: f.ids.message, conversationId: f.ids.conversation, contentVersion: "1", mediaDigest: "a".repeat(64), kind: "audio" }; const value = await f.store.savePlaybackCheckpoint(binding, 1, 8, true, () => true); return value === undefined && !(await f.read("playback-checkpoints|" + f.namespace)).entries.some(entry => entry.messageId === f.ids.message); }));
    await test("real IDB new-version first save succeeds and late old save or forget preserves it", () => page.evaluate(async () => { const f = audioTest, binding = { namespace: f.namespace, messageId: crypto.randomUUID(), conversationId: f.ids.conversation, contentVersion: "1", mediaDigest: "a".repeat(64), kind: "audio" }; await f.store.savePlaybackCheckpoint(binding, 1, 8, true, () => true); const next = { ...binding, contentVersion: "2", mediaDigest: "b".repeat(64) }; const saved = await f.store.savePlaybackCheckpoint(next, 2, 8, true, () => true); let rejected = false; try { await f.store.savePlaybackCheckpoint(binding, 3, 8, true, () => true); } catch { rejected = true; } await f.store.forgetPlaybackCheckpoint(binding, () => true); return saved.contentVersion === "2" && rejected && (await f.store.loadPlaybackCheckpoint(next, () => true)).mediaDigest === next.mediaDigest; }));
    await page.evaluate(() => { audioTest.locale = "bo"; audioTest.owner = "demo|tibetan-fixture"; document.documentElement.dataset.theme = "dark"; audioTest.renderRecorder(); }); await page.setViewportSize({ width: 390, height: 844 }); await fixture.locator("button").first().waitFor(); await fixture.screenshot({ path: path.join(out, "录音-藏文-黑夜.png") });
    await test("Tibetan narrow layout keeps visible recorder controls within the viewport", () => fixture.locator("button").evaluateAll(elements => elements.every(element => element.getBoundingClientRect().left >= 0 && element.getBoundingClientRect().right <= innerWidth + 1)));
    await page.evaluate(() => { audioTest.empty(); audioTest.mode = "normal"; window.fetch = audioTest.nativeFetch; document.getElementById("audio-fixture").style.display = "none"; document.getElementById("root").style.display = ""; });
    await page.setViewportSize({ width: 1280, height: 900 });
    const appRecorder = page.locator("#root .composer .audio-recorder"), audioBefore = await page.locator("#root .thread-body audio").count();
    await appRecorder.getByRole("button", { name: "录制语音", exact: true }).click(); await appRecorder.getByRole("button", { name: "停止并试听", exact: true }).waitFor(); await page.waitForTimeout(1200); await appRecorder.getByRole("button", { name: "停止并试听", exact: true }).click(); await appRecorder.locator("audio").waitFor();
    await test("actual App composer uses native recording and requires preview before its upload entry", async () => (await page.locator("#root .thread-body audio").count()) === audioBefore);
    await appRecorder.getByRole("button", { name: "发送这段录音", exact: true }).click(); await page.waitForFunction(count => document.querySelectorAll("#root .thread-body audio").length === count + 1, audioBefore);
    await test("actual App demo upload handoff displays the new audio message without a server write", async () => (await appRecorder.locator("audio").count()) === 0 && (await page.locator("#root .thread-body audio").count()) === audioBefore + 1);
    await page.locator("#root .composer").screenshot({ path: path.join(out, "App录音入口-中文-黑夜.png") });
    await page.evaluate(async () => { for (const item of audioTest.audioContexts ?? []) { item.oscillator.stop(); await item.audio.close(); } });
    const after = hashes(); assert.deepEqual(after, before); for (const file of sources) { const target = path.join(out, "输入源码", file); fs.mkdirSync(path.dirname(target), { recursive: true }); fs.copyFileSync(path.join(root, file), target); }
    fs.writeFileSync(path.join(out, "结果.json"), JSON.stringify({ startedAt, finishedAt: new Date().toISOString(), passed: checks.length, checks, sourceBefore: before, sourceAfter: after, nativeRecording: recorded, realMicrophone: false, realAPI: false, allHTTPFixtures: true, realIndexedDB: true, realMediaRecorder: true }, null, 2)); console.log(JSON.stringify({ out, passed: checks.length }));
  } catch (error) { fs.writeFileSync(path.join(out, "失败.json"), JSON.stringify({ startedAt, checks, error: String(error.stack ?? error), sourceBefore: before, sourceAfter: hashes() }, null, 2)); throw error; }
  finally { await browser?.close(); await vite.close(); }
})().catch(error => { console.error(error); process.exitCode = 1; });
