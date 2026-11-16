const fs = require("fs"),
  path = require("path"),
  https = require("https"),
  crypto = require("crypto"),
  assert = require("node:assert/strict");
const { chromium } = require(
  process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright",
);
const server = process.env.JOBSIM_SERVER_URL || "https://127.0.0.1:8443";
if (!process.env.JOBSIM_CA_FILE || !process.env.JOBSIM_TEST_CERT)
  throw Error("Provide the test CA and exact public server certificate");
const agent = new https.Agent({
  ca: fs.readFileSync(process.env.JOBSIM_CA_FILE),
});
const pin = crypto
  .createHash("sha256")
  .update(
    new crypto.X509Certificate(
      fs.readFileSync(process.env.JOBSIM_TEST_CERT),
    ).publicKey.export({ type: "spki", format: "der" }),
  )
  .digest("base64");
async function req(route, method = "GET", body, token) {
  if (
    method === "POST" &&
    /^\/conversations\/[^/]+\/messages$/.test(route) &&
    body &&
    !Object.hasOwn(body, "sendAdmission")
  ) {
    const conversationId = route.split("/")[2];
    const current = (
      await req("/conversations", "GET", undefined, token)
    ).items.find((item) => item.id === conversationId);
    assert.ok(
      current?.sendAdmission,
      "new native test intent requires a current admission snapshot",
    );
    body = { ...body, sendAdmission: current.sendAdmission };
  }
  return new Promise((resolve, reject) => {
    const r = https.request(
      server + "/api" + route,
      {
        method,
        agent,
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: "Bearer " + token } : {}),
        },
      },
      (res) => {
        let text = "";
        res.on("data", (s) => (text += s));
        res.on("end", () => {
          const j = JSON.parse(text || "{}");
          res.statusCode >= 400
            ? reject(Error(res.statusCode + " " + j.error?.code))
            : resolve(j);
        });
      },
    );
    r.on("error", reject);
    if (body) r.write(JSON.stringify(body));
    r.end();
  });
}
async function forwardVerified(request) {
  const headers = await request.allHeaders();
  assert.equal(new URL(request.url()).origin, new URL(server).origin);
  return new Promise((resolve, reject) => {
    const outgoing = https.request(
      request.url(),
      { method: request.method(), agent, headers },
      (response) => {
        response.resume();
        response.on("end", () => resolve(response.statusCode));
      },
    );
    outgoing.on("error", (error) =>
      reject(
        Error("Verified test forwarding failed: " + (error.code ?? "NETWORK")),
      ),
    );
    outgoing.setTimeout(8000, () =>
      outgoing.destroy(Error("Test forwarding timeout")),
    );
    outgoing.end(request.postDataBuffer() ?? undefined);
  });
}
(async () => {
  const stamp = Date.now().toString(36),
    a = "web_a_" + stamp,
    b = "web_b_" + stamp,
    password = "JobsIM " + stamp + " private test";
  const bob = await req("/auth/register", "POST", {
    username: b,
    password,
    requestId: crypto.randomUUID(),
    deviceName: "Web integration native peer",
  });
  await req("/me", "PATCH", { displayName: "联调对端" }, bob.token);
  const browser = await chromium.launch({
    headless: true,
    executablePath:
      process.env.BROWSER_EXECUTABLE ||
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    args: ["--ignore-certificate-errors-spki-list=" + pin],
  });
  const context = await browser.newContext({
    viewport: { width: 1440, height: 960 },
  });
  const page = await context.newPage();
  const errors = [];
  page.on("dialog", (dialog) => dialog.accept());
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("response", (r) => {
    if (r.url().includes("/api/"))
      console.log(new URL(r.url()).pathname, r.status());
  });
  let charlie;
  try {
    await page.goto(server);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.getByRole("button", { name: "注册", exact: true }).click();
    await page.locator("input[name=username]").fill(a);
    await page.locator("input[name=password]").fill(password);
    await page.locator("input[name=displayName]").fill("Web 联调用户");
    const registrationValidity = await page
      .locator(".modal form")
      .evaluate((form) => ({
        valid: form.checkValidity(),
        fields: Array.from(form.elements)
          .filter((item) => item.name)
          .map((item) => ({
            name: item.name,
            type: item.type,
            length: item.value.length,
            valid: item.validity.valid,
            reason: item.validationMessage,
          })),
      }));
    assert.ok(registrationValidity.valid, JSON.stringify(registrationValidity));
    await page
      .locator(".modal form")
      .getByRole("button", { name: "注册", exact: true })
      .click();
    await page
      .locator(".modal-overlay")
      .waitFor({ state: "hidden", timeout: 15000 });
    const me = await page.evaluate(async () => {
      const r = await fetch("/api/me", { credentials: "include" });
      return r.json();
    });
    assert.equal(me.username, a);
    const cookies = await context.cookies();
    assert.ok(cookies.some((c) => c.httpOnly && c.secure));
    const storage = await page.evaluate(() =>
      Object.entries(localStorage).filter(([k]) =>
        /token|refresh|password/i.test(k),
      ),
    );
    assert.deepEqual(storage, []);
    await page
      .locator(".sidebar")
      .getByRole("button", { name: "发起聊天", exact: true })
      .click();
    await page.locator(".modal .search input").fill(b);
    const person = page.locator(".person-choice").filter({ hasText: b });
    await person.waitFor();
    await person.getByRole("button", { name: "添加", exact: true }).click();
    await page.getByRole("status").getByText("申请已发送").waitFor();
    const r = await req("/contacts", undefined, undefined, bob.token);
    const invitation = r.requests.find(
      (x) => x.requesterId === me.id && x.state === "pending",
    );
    assert.ok(invitation);
    await req(
      "/contacts/requests/" + invitation.id + "/accept",
      "POST",
      {},
      bob.token,
    );
    await person.getByRole("button").first().click();
    await page.locator(".modal-overlay").waitFor({ state: "hidden" });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("真实浏览器到原生API的消息 " + stamp);
    const accepted = page.waitForResponse(
      (r) =>
        r.request().method() === "POST" &&
        /\/api\/conversations\/[^/]+\/messages$/.test(
          new URL(r.url()).pathname,
        ),
    );
    await page.getByRole("button", { name: "发送", exact: true }).click();
    assert.equal((await accepted).status(), 200);
    const conversations = await req(
      "/conversations",
      undefined,
      undefined,
      bob.token,
    );
    const c = conversations.items.find((x) => x.memberIds.includes(me.id));
    assert.ok(c);
    const history = await req(
      "/conversations/" + c.id + "/messages",
      undefined,
      undefined,
      bob.token,
    );
    assert.ok(
      history.items.some(
        (x) => x.text === "真实浏览器到原生API的消息 " + stamp,
      ),
    );
    const id = crypto.randomUUID();
    await req(
      "/conversations/" + c.id + "/messages",
      "POST",
      { clientMsgId: id, type: "text", text: "原生API回复 " + stamp },
      bob.token,
    );
    await page
      .locator(".thread-body")
      .getByText("原生API回复 " + stamp, { exact: true })
      .waitFor({ timeout: 12000 });
    let lostResponse = true;
    const retried = [];
    await page.route("**/api/conversations/*/messages", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const payload = route.request().postDataJSON();
      retried.push(payload.clientMsgId);
      if (lostResponse) {
        lostResponse = false;
        const durable = await page.evaluate(async (clientMsgId) => {
          const db = await new Promise((resolve, reject) => {
            const r = indexedDB.open("JobsIM", 1);
            r.onsuccess = () => resolve(r.result);
            r.onerror = () => reject(r.error);
          });
          return new Promise((resolve) => {
            const tx = db.transaction("spaces");
            const read = tx.objectStore("spaces").openCursor();
            let found = false;
            read.onsuccess = () => {
              const cursor = read.result;
              if (cursor) {
                if (
                  String(cursor.key).startsWith("outbox|") &&
                  cursor.value.some((m) => m.clientMsgId === clientMsgId)
                )
                  found = true;
                cursor.continue();
              }
            };
            tx.oncomplete = () => {
              db.close();
              resolve(found);
            };
          });
        }, payload.clientMsgId);
        assert.equal(durable, true, "Outbox must commit before HTTP");
        assert.equal(await forwardVerified(route.request()), 200);
        await route.abort("failed");
      } else await route.continue();
    });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("响应丢失后沿原ID核对 " + stamp);
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await page
      .locator(".message-meta")
      .getByRole("button", { name: "待发送", exact: true })
      .waitFor();
    await page
      .locator(".message-meta")
      .getByRole("button", { name: "待发送", exact: true })
      .click();
    await page
      .locator(".message-meta")
      .getByRole("button", { name: "待发送", exact: true })
      .waitFor({ state: "hidden" });
    assert.equal(retried.length, 2);
    assert.equal(retried[0], retried[1]);
    const reconciled = await req(
      "/conversations/" + c.id + "/messages",
      undefined,
      undefined,
      bob.token,
    );
    assert.equal(
      reconciled.items.filter((x) => x.text === "响应丢失后沿原ID核对 " + stamp)
        .length,
      1,
    );
    await page.unroute("**/api/conversations/*/messages");
    await page.reload();
    await page
      .locator(".thread-body")
      .getByText("原生API回复 " + stamp, { exact: true })
      .waitFor({ timeout: 12000 });
    const contextAgain = await page.evaluate(async () => {
      const r = await fetch("/api/auth/context", { credentials: "include" });
      return r.json();
    });
    assert.equal(contextAgain.user.id, me.id);
    assert.ok(contextAgain.deviceId);
    assert.ok(
      ["联调对端", b].includes(
        await page.locator(".thread-title h2").textContent(),
      ),
      "direct conversation must show its actual peer identity",
    );
    charlie = await req("/auth/register", "POST", {
      username: "web_c_" + stamp,
      password,
      requestId: crypto.randomUUID(),
      deviceName: "Web inbox test peer",
    });
    await req("/me", "PATCH", { displayName: "新申请人" }, charlie.token);
    await req(
      "/contacts/requests",
      "POST",
      { userId: me.id, requestId: crypto.randomUUID() },
      charlie.token,
    );
    await page.getByRole("button", { name: "联系人", exact: true }).click();
    const incoming = page
      .locator(".contact-request")
      .filter({ hasText: "新申请人" });
    await incoming.waitFor({ timeout: 12000 });
    assert.equal(
      await page.locator(".contact-request").count(),
      1,
      "outgoing and accepted records must not be actionable inbox items",
    );
    await incoming.getByRole("button", { name: "接受", exact: true }).click();
    await incoming.waitFor({ state: "hidden" });
    await page
      .locator(".sidebar")
      .getByRole("button", { name: /新申请人/ })
      .waitFor();
    const peerContacts = await req(
      "/contacts",
      undefined,
      undefined,
      charlie.token,
    );
    assert.ok(peerContacts.items.some((user) => user.id === me.id));
    await page.getByRole("button", { name: "消息", exact: true }).click();
    let reportUnknown = true;
    const reportIds = [];
    await page.route("**/api/reports", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const payload = route.request().postDataJSON();
      reportIds.push(payload.requestId);
      if (!reportUnknown) return route.continue();
      reportUnknown = false;
      const committed = await page.evaluate(async (requestId) => {
        const db = await new Promise((resolve) => {
          const r = indexedDB.open("JobsIM", 1);
          r.onsuccess = () => resolve(r.result);
        });
        return new Promise((resolve) => {
          let found = false;
          const tx = db.transaction("spaces"),
            read = tx.objectStore("spaces").openCursor();
          read.onsuccess = () => {
            const c = read.result;
            if (c) {
              if (
                String(c.key).endsWith("|report") &&
                c.value.requestId === requestId
              ) {
                found = true;
                if (c.value.label)
                  throw Error(
                    "Report operation persisted a source-body preview",
                  );
              }
              c.continue();
            }
          };
          tx.oncomplete = () => {
            db.close();
            resolve(found);
          };
        });
      }, payload.requestId);
      assert.equal(committed, true);
      assert.equal(await forwardVerified(route.request()), 200);
      await route.abort("failed");
    });
    const reportSource = page
      .locator(".thread-body article")
      .filter({ hasText: "真实浏览器到原生API的消息 " + stamp });
    await reportSource
      .getByRole("button", { name: "管理", exact: true })
      .click();
    await reportSource
      .getByRole("button", { name: "举报", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .locator("textarea[name=reason]")
      .fill("Web UI report " + stamp);
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "提交", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "重试", exact: true })
      .waitFor();
    await page.reload();
    await page.getByRole("button", { name: "设置", exact: true }).click();
    await page.getByRole("button", { name: "我的举报", exact: true }).click();
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "重试", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .locator(".report-form")
      .waitFor({ state: "hidden" });
    assert.equal(reportIds.length, 2);
    assert.equal(reportIds[0], reportIds[1]);
    await page
      .getByRole("dialog")
      .locator(".report-card")
      .filter({ hasText: "Web UI report " + stamp })
      .waitFor();
    const reports = await page.evaluate(() =>
      fetch("/api/reports", { credentials: "include" }).then((r) => r.json()),
    );
    assert.equal(
      reports.items.filter((item) => item.reason === "Web UI report " + stamp)
        .length,
      1,
    );
    await page.unroute("**/api/reports");
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    await page.getByRole("button", { name: "消息", exact: true }).click();
    const avatarBytes = await page.locator(".me-avatar").screenshot();
    let avatarUnknown = true;
    const avatarIds = [];
    await page.route("**/api/me/avatar", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const body = route.request().postDataBuffer();
      const requestId = body
        .toString("latin1")
        .match(/name="requestId"\r\n\r\n([^\r]+)/)?.[1];
      assert.ok(requestId);
      avatarIds.push(requestId);
      if (!avatarUnknown) return route.continue();
      avatarUnknown = false;
      const committed = await page.evaluate(async (id) => {
        const db = await new Promise((resolve, reject) => {
          const r = indexedDB.open("JobsIM", 1);
          r.onsuccess = () => resolve(r.result);
          r.onerror = () => reject(r.error);
        });
        return new Promise((resolve) => {
          let found = false;
          const tx = db.transaction("spaces"),
            read = tx.objectStore("spaces").openCursor();
          read.onsuccess = () => {
            const cursor = read.result;
            if (!cursor) return;
            if (
              String(cursor.key).startsWith("operation|") &&
              cursor.value.requestId === id
            ) {
              found = cursor.value.file instanceof File;
              window.__oldAvatarIntent = {
                key: cursor.key,
                value: cursor.value,
              };
            }
            cursor.continue();
          };
          tx.oncomplete = () => {
            db.close();
            resolve(found);
          };
        });
      }, requestId);
      assert.equal(
        committed,
        true,
        "avatar source and original ID must commit before HTTP",
      );
      assert.equal(await forwardVerified(route.request()), 200);
      await route.abort("failed");
    });
    await page.locator(".me-avatar").click();
    const profile = page.getByRole("dialog");
    await profile.locator("input[type=file]").setInputFiles({
      name: "own-logo.png",
      mimeType: "image/png",
      buffer: avatarBytes,
    });
    await profile.getByRole("button", { name: "重试", exact: true }).waitFor();
    await profile.getByRole("button", { name: "重试", exact: true }).click();
    await profile
      .getByRole("button", { name: "更换头像", exact: true })
      .waitFor();
    assert.equal(avatarIds.length, 2);
    assert.equal(avatarIds[0], avatarIds[1]);
    await page.unroute("**/api/me/avatar");
    const firstAvatar = await page.evaluate(() =>
      fetch("/api/me", { credentials: "include" }).then((r) => r.json()),
    );
    assert.ok(firstAvatar.avatarThumbnailURL);
    await page.waitForFunction(
      () =>
        document.querySelector(".avatar-actions button")?.disabled === false,
    );
    const nextPublication = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/me/avatar" &&
        r.request().method() === "POST",
    );
    await profile.locator("input[type=file]").setInputFiles({
      name: "own-logo-next.png",
      mimeType: "image/png",
      buffer: avatarBytes,
    });
    assert.equal((await nextPublication).status(), 200);
    await profile
      .getByRole("button", { name: "更换头像", exact: true })
      .waitFor();
    await page.waitForFunction(async (prior) => {
      const r = await fetch("/api/me", { credentials: "include" });
      return (await r.json()).profileVersion !== prior;
    }, firstAvatar.profileVersion);
    const currentAvatar = await page.evaluate(() =>
      fetch("/api/me", { credentials: "include" }).then((r) => r.json()),
    );
    const historical = await page.evaluate(
      (id) =>
        fetch("/api/me/avatar/operations/" + id, {
          credentials: "include",
        }).then((r) => r.json()),
      avatarIds[0],
    );
    assert.equal(historical.current, false);
    await page.evaluate(async () => {
      const fixture = window.__oldAvatarIntent;
      const db = await new Promise((resolve, reject) => {
        const r = indexedDB.open("JobsIM", 1);
        r.onsuccess = () => resolve(r.result);
        r.onerror = () => reject(r.error);
      });
      await new Promise((resolve, reject) => {
        const tx = db.transaction("spaces", "readwrite", {
          durability: "strict",
        });
        tx.objectStore("spaces").put(fixture.value, fixture.key);
        tx.oncomplete = resolve;
        tx.onerror = () => reject(tx.error);
      });
      db.close();
    });
    await page.reload();
    await page.locator(".me-avatar").waitFor();
    await page.waitForFunction(async () => {
      const db = await new Promise((resolve) => {
        const r = indexedDB.open("JobsIM", 1);
        r.onsuccess = () => resolve(r.result);
      });
      return new Promise((resolve) => {
        let pending = false;
        const tx = db.transaction("spaces"),
          read = tx.objectStore("spaces").openCursor();
        read.onsuccess = () => {
          const c = read.result;
          if (c) {
            if (String(c.key).endsWith("|avatar")) pending = true;
            c.continue();
          }
        };
        tx.oncomplete = () => {
          db.close();
          resolve(!pending);
        };
      });
    });
    await page.locator(".me-avatar").click();
    await page.getByRole("dialog").locator(".avatar img").waitFor();
    await page.waitForFunction(
      (url) =>
        document
          .querySelector("[role=dialog] .avatar img")
          ?.getAttribute("src") === url,
      currentAvatar.avatarThumbnailURL,
    );
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    await page
      .locator(".sidebar")
      .getByRole("button", { name: "发起聊天", exact: true })
      .click();
    await page
      .getByRole("dialog")
      .getByRole("checkbox", { name: "选择联系人 联调对端", exact: true })
      .check();
    await page
      .getByRole("dialog")
      .getByRole("checkbox", { name: "选择联系人 新申请人", exact: true })
      .check();
    await page
      .getByRole("dialog")
      .locator("[name=title]")
      .fill("Web group " + stamp);
    const createdGroup = page.waitForResponse(
      (r) =>
        new URL(r.url()).pathname === "/api/conversations" &&
        r.request().method() === "POST",
    );
    await page
      .getByRole("dialog")
      .getByRole("button", { name: "创建群聊", exact: true })
      .click();
    const group = await (await createdGroup).json();
    assert.equal(group.kind, "group");
    await page.locator(".modal-overlay").waitFor({ state: "hidden" });
    await page.locator(".thread-title").click();
    const groupPanel = page.locator(".group-controls");
    await groupPanel
      .getByRole("textbox", { name: "重命名群聊", exact: true })
      .fill("Renamed group " + stamp);
    await groupPanel.getByRole("button", { name: "保存", exact: true }).click();
    await page
      .locator(".thread-title h2")
      .filter({ hasText: "Renamed group " + stamp })
      .waitFor();
    const roster = (
      await req(
        "/conversations/" + group.id + "/members",
        undefined,
        undefined,
        bob.token,
      )
    ).items;
    const charlieGeneration = roster.find(
      (user) => user.id === charlie.user.id,
    ).membershipGeneration;
    let removalGeneration;
    page.on("request", (request) => {
      const url = new URL(request.url());
      if (
        request.method() === "DELETE" &&
        url.pathname ===
          "/api/conversations/" + group.id + "/members/" + charlie.user.id
      )
        removalGeneration = url.searchParams.get("membershipGeneration");
    });
    await groupPanel
      .getByRole("button", { name: "移除 新申请人", exact: true })
      .click();
    await groupPanel
      .locator(".member-row")
      .filter({ hasText: "新申请人" })
      .waitFor({ state: "hidden" });
    assert.equal(removalGeneration, charlieGeneration);
    const pendingId = await page.evaluate(
      async ({ groupId, userId }) => {
        const db = await new Promise((resolve) => {
          const request = indexedDB.open("JobsIM", 1);
          request.onsuccess = () => resolve(request.result);
        });
        return new Promise((resolve, reject) => {
          const tx = db.transaction("spaces", "readwrite", {
              durability: "strict",
            }),
            store = tx.objectStore("spaces"),
            read = store.openCursor();
          let pendingId;
          read.onsuccess = () => {
            const cursor = read.result;
            if (!cursor) return;
            const key = String(cursor.key);
            if (
              !key.startsWith("outbox|") &&
              !key.startsWith("operation|") &&
              key.includes(userId) &&
              cursor.value.conversations?.some((c) => c.id === groupId)
            ) {
              pendingId = crypto.randomUUID();
              const item = {
                id: pendingId,
                clientMsgId: pendingId,
                conversationId: groupId,
                senderId: userId,
                type: "text",
                text: "Frozen queue must never reappear after rejoin",
                createdAt: new Date().toISOString(),
                updatedAt: new Date().toISOString(),
                version: "1",
                sequence: "0",
                pending: true,
                restoreEpoch: "1",
                deleted: false,
                reactions: [],
                pinned: false,
                favorite: false,
              };
              const queue = store.get("outbox|" + key);
              queue.onsuccess = () =>
                store.put([...(queue.result ?? []), item], "outbox|" + key);
            }
            cursor.continue();
          };
          tx.oncomplete = () => {
            db.close();
            resolve(pendingId);
          };
          tx.onerror = () => reject(tx.error);
        });
      },
      { groupId: group.id, userId: me.id },
    );
    assert.ok(pendingId);
    await groupPanel
      .getByRole("combobox", { name: "转让群主", exact: true })
      .selectOption(bob.user.id);
    await groupPanel
      .getByRole("button", { name: "转让群主", exact: true })
      .click();
    await groupPanel
      .getByRole("button", { name: "退出群聊", exact: true })
      .waitFor();
    await page.screenshot({
      path: path.join(__dirname, "..", "报告", "Web-Group-HTTPS.png"),
    });
    await groupPanel
      .getByRole("button", { name: "退出群聊", exact: true })
      .click();
    await page.locator(".detail-panel").waitFor({ state: "hidden" });
    const leftRoster = (
      await req(
        "/conversations/" + group.id + "/members",
        undefined,
        undefined,
        bob.token,
      )
    ).items;
    assert.ok(!leftRoster.some((user) => user.id === me.id));
    const frozen = await page.evaluate(async (id) => {
      const db = await new Promise((resolve) => {
        const r = indexedDB.open("JobsIM", 1);
        r.onsuccess = () => resolve(r.result);
      });
      return new Promise((resolve) => {
        let result;
        const tx = db.transaction("spaces"),
          read = tx.objectStore("spaces").openCursor();
        read.onsuccess = () => {
          const c = read.result;
          if (c) {
            if (String(c.key).startsWith("outbox|"))
              result = c.value.find((m) => m.clientMsgId === id) ?? result;
            c.continue();
          }
        };
        tx.oncomplete = () => {
          db.close();
          resolve(result?.frozen === true);
        };
      });
    }, pendingId);
    assert.equal(frozen, true);
    await req(
      "/conversations/" + group.id + "/members",
      "POST",
      { userId: me.id },
      bob.token,
    );
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click({ timeout: 12000 });
    await page.locator(".thread-body").waitFor();
    assert.equal(
      await page
        .getByText("Frozen queue must never reappear after rejoin", {
          exact: true,
        })
        .count(),
      0,
    );
    await page.getByRole("button", { name: "设置", exact: true }).click();
    const unreadMessage = await req(
      "/conversations/" + group.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Background unread " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    const unreadBadge = page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .locator('[aria-label="未读 1"]');
    await unreadBadge.waitFor({ timeout: 12000 });
    await req(
      "/messages/" + unreadMessage.id,
      "PATCH",
      {
        text: "Edited background unread " + stamp,
        expectedVersion: unreadMessage.version,
        requestId: crypto.randomUUID(),
      },
      bob.token,
    );
    await page.waitForFunction(
      async ({ groupId, userId, id, stamp }) => {
        const db = await new Promise((resolve) => {
          const r = indexedDB.open("JobsIM", 1);
          r.onsuccess = () => resolve(r.result);
        });
        return new Promise((resolve) => {
          let ok = false;
          const tx = db.transaction("spaces"),
            read = tx.objectStore("spaces").openCursor();
          read.onsuccess = () => {
            const c = read.result;
            if (c) {
              const key = String(c.key);
              if (
                key.includes(userId) &&
                !key.startsWith("outbox|") &&
                !key.startsWith("operation|")
              ) {
                const ids = c.value.unread?.[groupId] ?? [];
                ok =
                  ids.length === 1 &&
                  ids[0] === id &&
                  c.value.messages?.[groupId]?.find((m) => m.id === id)
                    ?.text ===
                    "Edited background unread " + stamp;
              }
              c.continue();
            }
          };
          tx.oncomplete = () => {
            db.close();
            resolve(ok);
          };
        });
      },
      { groupId: group.id, userId: me.id, id: unreadMessage.id, stamp },
    );
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    await unreadBadge.waitFor({ state: "hidden" });
    await page.evaluate(() => {
      window.__realNotification = window.Notification;
      window.__noticeCalls = [];
      window.__noticeThrows = false;
      window.Notification = class {
        static permission = "granted";
        static async requestPermission() {
          return "granted";
        }
        constructor(title, options) {
          if (window.__noticeThrows) throw Error("Notification unavailable");
          window.__noticeCalls.push({ title, body: options.body });
        }
        close() {}
      };
    });
    await page.getByRole("button", { name: "设置", exact: true }).click();
    await page.getByRole("button", { name: /^通知/ }).click();
    await page.getByRole("button", { name: /^通知.*已开启/ }).waitFor();
    await page.evaluate(() => {
      Object.defineProperty(document, "visibilityState", {
        configurable: true,
        get: () => "hidden",
      });
      document.dispatchEvent(new Event("visibilitychange"));
    });
    await req(
      "/conversations/" + group.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Private notification body " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    await page.waitForFunction(() => window.__noticeCalls.length > 0);
    const notices = await page.evaluate(() => window.__noticeCalls);
    assert.ok(
      notices.every(
        (item) =>
          item.title === "JobsIM" &&
          !item.body.includes("Private notification body"),
      ),
    );
    await page.evaluate(() => {
      window.__noticeThrows = true;
    });
    const noNoticeMessage = await req(
      "/conversations/" + group.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Persists despite notification failure " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    await page.waitForFunction(
      async ({ userId, groupId, messageId }) => {
        const db = await new Promise((resolve) => {
          const r = indexedDB.open("JobsIM", 1);
          r.onsuccess = () => resolve(r.result);
        });
        return new Promise((resolve) => {
          let found = false;
          const tx = db.transaction("spaces"),
            read = tx.objectStore("spaces").openCursor();
          read.onsuccess = () => {
            const c = read.result;
            if (c) {
              if (
                String(c.key).includes(userId) &&
                !String(c.key).startsWith("outbox|") &&
                !String(c.key).startsWith("operation|")
              )
                found = !!c.value.messages?.[groupId]?.some(
                  (m) => m.id === messageId,
                );
              c.continue();
            }
          };
          tx.oncomplete = () => {
            db.close();
            resolve(found);
          };
        });
      },
      { userId: me.id, groupId: group.id, messageId: noNoticeMessage.id },
    );
    await page.evaluate(() => {
      delete document.visibilityState;
      window.Notification = window.__realNotification;
      document.dispatchEvent(new Event("visibilitychange"));
    });
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    // A second device removes and re-adds this member while an original intent remains unsent.
    let stalePayload;
    await page.route("**/api/conversations/*/messages", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      stalePayload = route.request().postDataJSON();
      await route.abort("failed");
    });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("Old membership pending " + stamp);
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await page.getByRole("button", { name: "重试", exact: true }).waitFor();
    assert.ok(stalePayload?.sendAdmission);
    await page.unroute("**/api/conversations/*/messages");
    const memberBefore = (
      await req(
        "/conversations/" + group.id + "/members",
        "GET",
        undefined,
        bob.token,
      )
    ).items.find((item) => item.id === me.id);
    await req(
      "/conversations/" +
        group.id +
        "/members/" +
        me.id +
        "?membershipGeneration=" +
        encodeURIComponent(memberBefore.membershipGeneration),
      "DELETE",
      undefined,
      bob.token,
    );
    await req(
      "/conversations/" + group.id + "/members",
      "POST",
      { userId: me.id },
      bob.token,
    );
    const refused = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname ===
          "/api/conversations/" + group.id + "/messages",
    );
    await page.getByRole("button", { name: "重试", exact: true }).click();
    assert.equal((await refused).status(), 409);
    await page
      .getByRole("status")
      .getByText(/发送资格已变更/)
      .waitFor();
    const oldIntent = await page.evaluate(async (id) => {
      const db = await new Promise((resolve) => {
        const request = indexedDB.open("JobsIM", 1);
        request.onsuccess = () => resolve(request.result);
      });
      return new Promise((resolve) => {
        let item;
        const tx = db.transaction("spaces"),
          read = tx.objectStore("spaces").openCursor();
        read.onsuccess = () => {
          const cursor = read.result;
          if (cursor) {
            if (String(cursor.key).startsWith("outbox|"))
              item = cursor.value.find((row) => row.clientMsgId === id) ?? item;
            cursor.continue();
          }
        };
        tx.oncomplete = () => {
          db.close();
          resolve(item);
        };
      });
    }, stalePayload.clientMsgId);
    assert.equal(oldIntent.frozen, true);
    assert.equal(oldIntent.sendAdmission, stalePayload.sendAdmission);
    assert.equal(oldIntent.freezeReason, "SEND_ADMISSION_CHANGED");
    await page.waitForTimeout(3400);
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("New membership message " + stamp);
    const newSend = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname ===
          "/api/conversations/" + group.id + "/messages",
    );
    await page.getByRole("button", { name: "发送", exact: true }).click();
    const fresh = await newSend;
    assert.equal(fresh.status(), 200);
    assert.notEqual(
      fresh.request().postDataJSON().sendAdmission,
      stalePayload.sendAdmission,
    );
    const afterCycle = (
      await req(
        "/conversations/" + group.id + "/messages",
        "GET",
        undefined,
        bob.token,
      )
    ).items;
    assert.ok(
      !afterCycle.some((item) => item.clientMsgId === stalePayload.clientMsgId),
    );

    // Both lookup lists load more than one server page and recheck the exact source before opening it.
    const lookupMessages = [];
    for (let index = 0; index < 51; index++) {
      lookupMessages.push(
        await req(
          "/conversations/" + group.id + "/messages",
          "POST",
          {
            clientMsgId: crypto.randomUUID(),
            type: "text",
            text: "Lookup fixture " + stamp + " " + index,
            restoreEpoch: "1",
          },
          bob.token,
        ),
      );
    }
    await page.evaluate(
      async (ids) => {
        const csrf = await (
          await fetch("/api/auth/csrf", { credentials: "include" })
        ).json();
        for (const id of ids) {
          const response = await fetch("/api/messages/" + id + "/favorite", {
            method: "PUT",
            credentials: "include",
            headers: {
              "X-JobsIM-Client": "web",
              "X-CSRF-Token": csrf.csrfToken,
            },
          });
          if (!response.ok)
            throw Error("Favorite fixture setup failed: " + response.status);
        }
      },
      lookupMessages.map((message) => message.id),
    );
    await page
      .locator("nav")
      .getByRole("button", { name: "收藏", exact: true })
      .click();
    await page.waitForFunction(
      () => document.querySelectorAll(".sidebar .saved-row").length === 50,
    );
    await page
      .locator(".sidebar")
      .getByRole("button", { name: "加载更多", exact: true })
      .click();
    await page.waitForFunction(
      () => document.querySelectorAll(".sidebar .saved-row").length === 51,
    );
    const exactSource = lookupMessages.at(-1);
    const sourceCheck = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname ===
          "/api/messages/" + exactSource.id &&
        response.request().method() === "GET",
    );
    await page
      .locator(".sidebar .saved-row")
      .filter({ hasText: exactSource.text })
      .click();
    assert.equal((await sourceCheck).status(), 200);
    await page.locator("#message-" + exactSource.id).waitFor();
    await page
      .locator(".thread-header")
      .getByRole("button", { name: "搜索消息或联系人", exact: true })
      .click();
    await page.locator(".modal input[name=q]").fill("Lookup fixture " + stamp);
    await page
      .locator(".modal form")
      .getByRole("button", { name: "搜索消息或联系人", exact: true })
      .click();
    await page.waitForFunction(
      () => document.querySelectorAll(".modal .saved-row").length === 50,
    );
    await page
      .locator(".modal")
      .getByRole("button", { name: "加载更多", exact: true })
      .click();
    await page.waitForFunction(
      () => document.querySelectorAll(".modal .saved-row").length === 51,
    );
    await page.screenshot({
      path: path.join(__dirname, "..", "报告", "Web-Lookup-HTTPS.png"),
    });
    const withdrawn = lookupMessages[0];
    await req("/messages/" + withdrawn.id, "DELETE", undefined, bob.token);
    const withdrawnCheck = page.waitForResponse(
      (response) =>
        new URL(response.url()).pathname === "/api/messages/" + withdrawn.id &&
        response.request().method() === "GET",
    );
    await page
      .locator(".modal .saved-row")
      .filter({ hasText: withdrawn.text })
      .click();
    assert.equal((await withdrawnCheck).status(), 200);
    await page
      .locator("#message-" + withdrawn.id)
      .getByText("撤回", { exact: true })
      .waitFor();
    assert.ok(
      !(await page.locator("#message-" + withdrawn.id).textContent()).includes(
        withdrawn.text,
      ),
    );

    // Relationship actions show only the current user's own gate and keep the original reopen ID after a lost response.
    await page
      .locator(".conversation-row")
      .filter({ hasText: "联调对端" })
      .click();
    await page.getByRole("button", { name: "会话详情", exact: true }).click();
    let peerPanel = page.locator(".peer-controls");
    await peerPanel
      .getByText("允许对方向我发起新申请", { exact: true })
      .waitFor();
    await peerPanel.getByRole("button", { name: "拉黑", exact: true }).click();
    await peerPanel
      .getByRole("button", { name: "解除拉黑", exact: true })
      .waitFor();
    assert.equal(
      (
        await req(
          "/contacts/" + me.id + "/relationship",
          "GET",
          undefined,
          bob.token,
        )
      ).blockedByMe,
      false,
    );
    await peerPanel
      .getByRole("button", { name: "解除拉黑", exact: true })
      .click();
    await peerPanel
      .getByRole("button", { name: "允许对方重新申请", exact: true })
      .waitFor();
    const reopenIds = [];
    let loseReopen = true;
    await page.route("**/api/contacts/*/reopen", async (route) => {
      const payload = route.request().postDataJSON();
      reopenIds.push(payload.requestId);
      if (loseReopen) {
        loseReopen = false;
        assert.equal(await forwardVerified(route.request()), 200);
        await route.abort("failed");
      } else await route.continue();
    });
    await peerPanel
      .getByRole("button", { name: "允许对方重新申请", exact: true })
      .click();
    await peerPanel
      .getByRole("button", { name: /重试.*允许对方重新申请/ })
      .waitFor();
    await page.reload();
    await page
      .locator(".conversation-row")
      .filter({ hasText: "联调对端" })
      .click();
    await page.getByRole("button", { name: "会话详情", exact: true }).click();
    peerPanel = page.locator(".peer-controls");
    await peerPanel
      .getByRole("button", { name: /重试.*允许对方重新申请/ })
      .click();
    await peerPanel
      .getByRole("button", { name: "停止重试", exact: true })
      .waitFor({ state: "hidden" });
    assert.equal(reopenIds.length, 2);
    assert.equal(reopenIds[0], reopenIds[1]);
    await page.unroute("**/api/contacts/*/reopen");
    await peerPanel
      .getByText("允许对方向我发起新申请", { exact: true })
      .waitFor();
    await peerPanel
      .getByRole("button", { name: "添加", exact: true })
      .waitFor();
    await req(
      "/contacts/" + me.id + "/reopen",
      "POST",
      { requestId: crypto.randomUUID() },
      bob.token,
    );
    await peerPanel.getByRole("button", { name: "添加", exact: true }).click();
    await page
      .getByRole("status")
      .getByText("申请已发送", { exact: true })
      .waitFor();
    const renewed = (
      await req("/contacts", "GET", undefined, bob.token)
    ).requests.find(
      (item) => item.requesterId === me.id && item.state === "pending",
    );
    assert.ok(renewed);
    await req(
      "/contacts/requests/" + renewed.id + "/accept",
      "POST",
      {},
      bob.token,
    );
    await peerPanel
      .getByRole("button", { name: "重新加载", exact: true })
      .click();
    await peerPanel
      .getByRole("button", { name: "移除", exact: true })
      .waitFor();
    await peerPanel.getByRole("button", { name: "移除", exact: true }).click();
    await peerPanel
      .getByRole("button", { name: "添加", exact: true })
      .waitFor();
    assert.equal(
      (
        await req(
          "/contacts/" + me.id + "/relationship",
          "GET",
          undefined,
          bob.token,
        )
      ).friend,
      false,
    );
    await page.getByRole("button", { name: "发送", exact: true }).waitFor();
    await page.waitForFunction(
      () => document.querySelector(".send-button")?.disabled === true,
    );
    // Select structured recipients explicitly; an edited incoming mention never increments the count again.
    await req(
      "/conversations/" + group.id,
      "PATCH",
      { ownerId: me.id },
      bob.token,
    );
    await page.waitForTimeout(3300);
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    await page.getByRole("button", { name: "提及成员", exact: true }).click();
    await page.getByRole("checkbox", { name: "联调对端", exact: true }).check();
    await page
      .locator(".modal")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    const mentionBodies = [];
    let loseMention = true,
      finishMention;
    const mentionCommitted = new Promise((resolve) => {
      finishMention = resolve;
    });
    const mentionHistoryPattern =
      "**/api/conversations/" + group.id + "/messages?*";
    const holdMentionHistory = (route) =>
      route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          items: [],
          cursor:
            new URL(route.request().url()).searchParams.get("after") || "1:0",
        }),
      });
    await page.route(mentionHistoryPattern, holdMentionHistory);
    await page.route("**/api/conversations/*/messages", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const body = route.request().postDataJSON();
      mentionBodies.push(body);
      assert.deepEqual(body.mentionedUserIds, [bob.user.id]);
      if (loseMention) {
        loseMention = false;
        assert.equal(await forwardVerified(route.request()), 200);
        await route.abort("failed");
        finishMention();
      } else await route.continue();
    });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("Structured mention outbound " + stamp);
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await mentionCommitted;
    const mentionRetried = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname ===
          "/api/conversations/" + group.id + "/messages" &&
        response.status() === 200,
    );
    await page.getByRole("button", { name: "重试", exact: true }).click();
    await mentionRetried;
    await page.unroute(mentionHistoryPattern, holdMentionHistory);
    await page.unroute("**/api/conversations/*/messages");
    assert.equal(mentionBodies.length, 2);
    assert.deepEqual(mentionBodies[0], mentionBodies[1]);
    const confirmedMention = await (await mentionRetried).json();
    const outgoingMention = await req(
      "/messages/" + confirmedMention.id,
      "GET",
      undefined,
      bob.token,
    );
    assert.equal(outgoingMention.mentionedMe, true);
    await page.getByRole("button", { name: "设置", exact: true }).click();
    await req(
      "/conversations/" + group.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "@Web 联调用户 plain text " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    const targetedMention = await req(
      "/conversations/" + group.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Structured incoming mention " + stamp,
        mentionedUserIds: [me.id],
        restoreEpoch: "1",
      },
      bob.token,
    );
    const mentionBadge = page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .locator(".mention-badge");
    await mentionBadge.waitFor();
    assert.equal(await mentionBadge.getAttribute("aria-label"), "未读提及 1");
    await req(
      "/messages/" + targetedMention.id,
      "PATCH",
      {
        text: "Edited incoming mention " + stamp,
        expectedVersion: targetedMention.version,
        requestId: crypto.randomUUID(),
      },
      bob.token,
    );
    await page.waitForTimeout(3300);
    assert.equal(await mentionBadge.getAttribute("aria-label"), "未读提及 1");
    await req("/blocks/" + me.id, "PUT", undefined, bob.token);
    await mentionBadge.waitFor({ state: "hidden" });
    await req("/blocks/" + me.id, "DELETE", undefined, bob.token);
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    await mentionBadge.waitFor({ state: "hidden" });

    // The announcement is a reference to the content version, with a durable original operation and no stored source body.
    const sourceRow = page.locator("#message-" + outgoingMention.id);
    await sourceRow.hover();
    await sourceRow.getByRole("button", { name: "管理", exact: true }).click();
    await sourceRow
      .getByRole("button", { name: "设为群公告", exact: true })
      .click();
    await page
      .locator(".modal .announcement-panel")
      .getByRole("combobox", { name: "有效期", exact: true })
      .selectOption("1");
    const announcementBodies = [];
    let finishAnnouncement;
    const announcementCommitted = new Promise((resolve) => {
      finishAnnouncement = resolve;
    });
    await page.route("**/api/conversations/*/announcement", async (route) => {
      if (route.request().method() !== "PUT") return route.continue();
      const payload = route.request().postDataJSON();
      announcementBodies.push(payload);
      assert.equal(payload.sourceVersion, outgoingMention.contentVersion);
      assert.equal(await forwardVerified(route.request()), 200);
      await route.abort("failed");
      finishAnnouncement();
    });
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "设为群公告", exact: true })
      .click();
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "重试", exact: true })
      .waitFor();
    // Pending UI appears before the HTTP call; reload only after the simulated lost response.
    await announcementCommitted;
    await page.reload();
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Renamed group " + stamp })
      .click();
    await page.getByRole("button", { name: "会话详情", exact: true }).click();
    const announcementPanel = page.locator(".detail-panel .announcement-panel");
    await announcementPanel
      .getByRole("button", { name: "重试", exact: true })
      .click();
    await announcementPanel
      .getByRole("button", { name: "停止重试", exact: true })
      .waitFor({ state: "hidden" });
    await page.unroute("**/api/conversations/*/announcement");
    assert.equal(
      announcementBodies.length,
      1,
      "completed announcement is queried rather than republished",
    );
    await announcementPanel
      .locator(".announcement-source")
      .getByText("Structured mention outbound " + stamp, { exact: true })
      .waitFor();
    await req(
      "/messages/" + outgoingMention.id + "/favorite",
      "PUT",
      undefined,
      bob.token,
    );
    await page.waitForTimeout(3300);
    await announcementPanel.locator(".announcement-source").waitFor();
    await page.screenshot({
      path: path.join(__dirname, "..", "报告", "Web-Announcement-HTTPS.png"),
    });
    await page.evaluate(
      async ({ id, text }) => {
        const csrf = await (
          await fetch("/api/auth/csrf", { credentials: "include" })
        ).json();
        const message = await (
          await fetch("/api/messages/" + id, { credentials: "include" })
        ).json();
        const response = await fetch("/api/messages/" + id, {
          method: "PATCH",
          credentials: "include",
          headers: {
            "Content-Type": "application/json",
            "X-JobsIM-Client": "web",
            "X-CSRF-Token": csrf.csrfToken,
          },
          body: JSON.stringify({
            text,
            expectedVersion: message.version,
            requestId: crypto.randomUUID(),
          }),
        });
        if (!response.ok)
          throw Error("Announcement source edit failed: " + response.status);
      },
      { id: outgoingMention.id, text: "Edited announcement source " + stamp },
    );
    await announcementPanel
      .getByText("公告原消息不可用", { exact: true })
      .waitFor();
    await announcementPanel
      .locator(".announcement-source")
      .waitFor({ state: "hidden" });
    await announcementPanel
      .getByRole("button", { name: "清除公告", exact: true })
      .click();
    await announcementPanel.getByText("暂无公告", { exact: true }).waitFor();
    // Losing and regaining ownership never grants an earlier unsent privileged intent a new role qualification.
    const transferAwayAndBack = async () => {
      await page.evaluate(
        async ({ groupId, peerId }) => {
          const csrf = await (
            await fetch("/api/auth/csrf", { credentials: "include" })
          ).json();
          const response = await fetch("/api/conversations/" + groupId, {
            method: "PATCH",
            credentials: "include",
            headers: {
              "Content-Type": "application/json",
              "X-JobsIM-Client": "web",
              "X-CSRF-Token": csrf.csrfToken,
            },
            body: JSON.stringify({ ownerId: peerId }),
          });
          if (!response.ok)
            throw Error("Owner transfer fixture failed: " + response.status);
        },
        { groupId: group.id, peerId: bob.user.id },
      );
      await req(
        "/conversations/" + group.id,
        "PATCH",
        { ownerId: me.id },
        bob.token,
      );
    };
    await page.getByRole("button", { name: "会话详情", exact: true }).click();
    await page.getByRole("button", { name: "提及成员", exact: true }).click();
    await page.getByRole("checkbox", { name: "提及全体", exact: true }).check();
    await page
      .locator(".modal")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    let oldAll, finishOldAll;
    const oldAllCaptured = new Promise((resolve) => {
      finishOldAll = resolve;
    });
    await page.route("**/api/conversations/*/messages", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      oldAll = route.request().postDataJSON();
      await route.abort("failed");
      finishOldAll();
    });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("Never revive old owner intent " + stamp);
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await page.getByRole("button", { name: "重试", exact: true }).waitFor();
    await oldAllCaptured;
    await page.unroute("**/api/conversations/*/messages");
    assert.equal(oldAll.mentionAll, true);
    await transferAwayAndBack();
    const oldAllRejected = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        new URL(response.url()).pathname ===
          "/api/conversations/" + group.id + "/messages" &&
        response.request().postDataJSON().clientMsgId === oldAll.clientMsgId,
    );
    await page.getByRole("button", { name: "重试", exact: true }).click();
    assert.equal((await oldAllRejected).status(), 409);
    await page
      .getByRole("status")
      .getByText(/发送资格已变更/)
      .waitFor();
    const allResult = await page.evaluate(
      async ({ groupId, payload }) => {
        const csrf = await (
          await fetch("/api/auth/csrf", { credentials: "include" })
        ).json();
        const response = await fetch(
          "/api/conversations/" + groupId + "/message-results",
          {
            method: "POST",
            credentials: "include",
            headers: {
              "Content-Type": "application/json",
              "X-JobsIM-Client": "web",
              "X-CSRF-Token": csrf.csrfToken,
            },
            body: JSON.stringify(payload),
          },
        );
        return response.status;
      },
      { groupId: group.id, payload: oldAll },
    );
    assert.equal(allResult, 404);
    await page.waitForTimeout(3300);
    const refreshedSource = page.locator("#message-" + outgoingMention.id);
    await refreshedSource.hover();
    await refreshedSource
      .getByRole("button", { name: "管理", exact: true })
      .click();
    await refreshedSource
      .getByRole("button", { name: "设为群公告", exact: true })
      .click();
    let oldAnnouncement, finishOldAnnouncement;
    const oldAnnouncementCaptured = new Promise((resolve) => {
      finishOldAnnouncement = resolve;
    });
    await page.route("**/api/conversations/*/announcement", async (route) => {
      if (route.request().method() !== "PUT") return route.continue();
      oldAnnouncement = route.request().postDataJSON();
      await route.abort("failed");
      finishOldAnnouncement();
    });
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "设为群公告", exact: true })
      .click();
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "重试", exact: true })
      .waitFor();
    await oldAnnouncementCaptured;
    await page.unroute("**/api/conversations/*/announcement");
    assert.ok(oldAnnouncement.roleGeneration);
    await transferAwayAndBack();
    const oldAnnouncementRejected = page.waitForResponse(
      (response) =>
        response.request().method() === "PUT" &&
        new URL(response.url()).pathname ===
          "/api/conversations/" + group.id + "/announcement",
    );
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "重试", exact: true })
      .click();
    assert.equal((await oldAnnouncementRejected).status(), 409);
    await page
      .locator(".modal .announcement-panel")
      .getByRole("button", { name: "重试", exact: true })
      .waitFor({ state: "hidden" });
    assert.equal(
      (
        await req(
          "/conversations/" + group.id + "/announcement",
          "GET",
          undefined,
          bob.token,
        )
      ).state,
      "empty",
    );
    await page
      .locator(".modal")
      .getByRole("button", { name: "关闭", exact: true })
      .click();
    const report = path.join(__dirname, "..", "报告");
    await page.screenshot({ path: path.join(report, "Web-Live-HTTPS.png") });
    assert.deepEqual(errors, []);
    fs.writeFileSync(
      path.join(report, "integration-result.json"),
      JSON.stringify(
        {
          status: "passed",
          at: new Date().toISOString(),
          checks: [
            "strict CA native peer",
            "exact certificate pin isolated browser",
            "HttpOnly Secure session",
            "no JS tokens",
            "registration and browser confirmation",
            "friend request acceptance",
            "unobserved owner-to-member-to-owner cannot revive an unsent at-all message or announcement; original role snapshot is never replaced, read-only lookup confirms no new message",
            "structured member selection persists in the original retry payload; plain at-text has no mention, incoming edit does not increment mention count, opening clears local mentions",
            "announcement original operation survives lost response and restart, completed operation is queried only, personal favorite preserves the announcement, content edit invalidates its preview, owner clears with version checks",
            "server favorites and scoped search load 50 plus one unique result, signed cursors remain opaque, clicked source is rechecked and a recalled result never shows its old body",
            "unobserved remote member exit/rejoin rejects original unsent admission and freezes the exact persisted payload; new intent uses new admission",
            "own relationship GET drives block/unblock/remove/re-request UI; original reopen identity survives lost response and browser restart without projecting peer private gates",
            "browser notification protocol default excludes source body; notification constructor failure does not prevent real incoming message persistence (OS delivery not tested)",
            "local unread identities persist with incoming pages, edits do not count again, opening the visible conversation clears them",
            "real group creation/rename/member-generation removal/owner transfer/leave through UI; late committed Outbox frozen on leave and hidden after rejoin",
            "report source identity committed before HTTP; browser refresh retries original operation and creates one report without persisting the source-body preview",
            "incoming request shows authorized requester profile; only pending recipient items are actionable; UI acceptance refreshes real friendship",
            "browser/native API bidirectional messages",
            "refresh retains real account and history",
            "lost response retries original ID without duplicate message",
            "avatar source and identity committed before HTTPS; lost response original-ID retry",
            "historical avatar result current=false never rolls back the current profile",
          ],
          errors,
        },
        null,
        2,
      ),
    );
    console.log("PASS Web HTTPS and real PostgreSQL bidirectional integration");
  } catch (error) {
    console.error(
      "Browser form diagnostics",
      JSON.stringify(
        await page.locator(".modal form").evaluateAll((forms) =>
          forms.map((form) => ({
            valid: form.checkValidity(),
            controls: Array.from(form.elements)
              .filter((item) => item.name)
              .map((item) => ({
                name: item.name,
                valid: item.validity.valid,
                reason: item.validationMessage,
                disabled: item.disabled,
                length: item.value.length,
              })),
          })),
        ),
      ),
    );
    console.error(
      "Browser notices",
      await page.getByRole("status").allTextContents(),
    );
    throw error;
  } finally {
    await page
      .evaluate(async () => {
        try {
          const r = await fetch("/api/auth/csrf", { credentials: "include" });
          if (r.ok) {
            const { csrfToken } = await r.json();
            await fetch("/api/auth/logout", {
              method: "POST",
              credentials: "include",
              headers: {
                "X-JobsIM-Client": "web",
                "X-CSRF-Token": csrfToken,
                "Content-Type": "application/json",
              },
              body: "{}",
            });
          }
        } catch {}
      })
      .catch(() => {});
    await req("/auth/logout", "POST", {}, bob.token).catch(() => {});
    if (charlie)
      await req("/auth/logout", "POST", {}, charlie.token).catch(() => {});
    await browser.close();
  }
})().catch((e) => {
  console.error(e.stack ?? String(e));
  process.exitCode = 1;
});
