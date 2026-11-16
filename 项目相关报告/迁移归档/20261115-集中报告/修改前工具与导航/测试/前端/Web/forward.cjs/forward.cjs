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
async function captureVerified(request) {
  const headers = await request.allHeaders();
  return new Promise((resolve, reject) => {
    const outgoing = https.request(
      request.url(),
      { method: request.method(), agent, headers },
      (response) => {
        const chunks = [];
        response.on("data", (chunk) => chunks.push(chunk));
        response.on("end", () =>
          resolve({ status: response.statusCode, body: Buffer.concat(chunks) }),
        );
      },
    );
    outgoing.on("error", reject);
    outgoing.setTimeout(8000, () =>
      outgoing.destroy(Error("Forward capture timeout")),
    );
    outgoing.end(request.postDataBuffer() ?? undefined);
  });
}
(async () => {
  const bootstrap = await req("/bootstrap");
  assert.equal(bootstrap.features.forwarding, true);
  const stamp = Date.now().toString(36),
    password = "JobsIM " + stamp + " private test";
  const a = await req("/auth/register", "POST", {
    username: "webforward_a_" + stamp,
    password,
    requestId: crypto.randomUUID(),
    deviceName: "Forward native setup",
  });
  const bob = await req("/auth/register", "POST", {
    username: "webforward_b_" + stamp,
    password,
    requestId: crypto.randomUUID(),
    deviceName: "Forward native peer",
  });
  await req(
    "/me",
    "PATCH",
    { displayName: "Forward Peer " + stamp },
    bob.token,
  );
  const invitation = await req(
    "/contacts/requests",
    "POST",
    { userId: bob.user.id, requestId: crypto.randomUUID() },
    a.token,
  );
  await req(
    "/contacts/requests/" + invitation.id + "/accept",
    "POST",
    {},
    bob.token,
  );
  const origin = await req(
    "/conversations",
    "POST",
    {
      kind: "group",
      title: "Forward Origin " + stamp,
      memberIds: [bob.user.id],
      requestId: crypto.randomUUID(),
    },
    a.token,
  );
  const destination = await req(
    "/conversations",
    "POST",
    {
      kind: "direct",
      memberIds: [bob.user.id],
      requestId: crypto.randomUUID(),
    },
    a.token,
  );
  const source = await req(
    "/conversations/" + origin.id + "/messages",
    "POST",
    {
      clientMsgId: crypto.randomUUID(),
      type: "text",
      text: "Forward reference source " + stamp,
      restoreEpoch: "1",
    },
    bob.token,
  );
  const browser = await chromium.launch({
    headless: true,
    executablePath:
      process.env.BROWSER_EXECUTABLE ||
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    args: ["--ignore-certificate-errors-spki-list=" + pin],
  });
  const page = await browser.newPage({
    viewport: { width: 1440, height: 960 },
  });
  // The isolated API fixture has no WebDir; serve the actual built files at the same HTTPS origin.
  const dist = path.resolve(__dirname, "../../../../代码/前端/Web/Dist");
  await page.route("**/*", (route) => {
    const url = new URL(route.request().url());
    if (url.origin !== server || url.pathname.startsWith("/api/"))
      return route.continue();
    const relative =
      url.pathname === "/"
        ? "index.html"
        : decodeURIComponent(url.pathname.slice(1));
    const file = path.resolve(dist, relative);
    if (!file.startsWith(dist + path.sep) || !fs.existsSync(file))
      return route.continue();
    const mime =
      {
        ".html": "text/html",
        ".js": "text/javascript",
        ".css": "text/css",
        ".svg": "image/svg+xml",
        ".png": "image/png",
      }[path.extname(file)] ?? "application/octet-stream";
    return route.fulfill({
      status: 200,
      contentType: mime,
      body: fs.readFileSync(file),
    });
  });
  page.on("dialog", (d) => d.accept());
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  page.on("response", (response) => {
    if (
      response.url().includes("/api/") &&
      response.request().method() !== "GET"
    )
      console.log(
        response.request().method(),
        new URL(response.url()).pathname,
        response.status(),
      );
  });
  const records = [],
    lookups = [];
  let raceSource, raceCompleted;
  const lateRace = new Promise((resolve) => {
    raceCompleted = resolve;
  });
  let committed;
  const commit = new Promise((resolve) => (committed = resolve));
  let hold = true,
    faultMode = "commit",
    neverFinished;
  const neverArrived = new Promise((resolve) => {
    neverFinished = resolve;
  });
  await page.route(
    "**/api/conversations/" + destination.id + "/messages?*",
    (route) =>
      hold
        ? route.fulfill({
            status: 200,
            contentType: "application/json",
            body: JSON.stringify({
              items: [],
              cursor:
                new URL(route.request().url()).searchParams.get("after") === "0"
                  ? "1:0"
                  : (new URL(route.request().url()).searchParams.get("after") ??
                    "1:0"),
            }),
          })
        : route.continue(),
  );
  await page.route("**/api/conversations/*/messages", async (route) => {
    if (
      route.request().method() !== "POST" ||
      route.request().postDataJSON()?.type !== "forward"
    )
      return route.continue();
    records.push(route.request().postDataJSON());
    if (faultMode === "race") {
      const old = await captureVerified(route.request());
      assert.equal(old.status, 200);
      const accepted = JSON.parse(old.body);
      await req(
        "/messages/" + raceSource.id,
        "PATCH",
        {
          text: "Race source changed " + stamp,
          expectedVersion: raceSource.version,
          requestId: crypto.randomUUID(),
        },
        bob.token,
      );
      await page
        .locator("#message-" + accepted.id)
        .getByText("内容暂不可用", { exact: true })
        .waitFor({ timeout: 12000 });
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: old.body,
      });
      raceCompleted(accepted);
      return;
    }

    if (faultMode === "never") {
      faultMode = "retry";
      await route.abort("failed");
      neverFinished();
      return;
    }
    if (faultMode === "retry" || faultMode === "plain") return route.continue();
    assert.equal(await forwardVerified(route.request()), 200);
    await route.abort("failed");
    committed();
  });
  await page.route("**/api/conversations/*/message-results", async (route) => {
    lookups.push(route.request().postDataJSON());
    return route.continue();
  });
  try {
    await page.goto(server);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.locator("input[name=username]").fill(a.user.username);
    await page.locator("input[name=password]").fill(password);
    await page
      .locator(".modal form")
      .getByRole("button", { name: "登录", exact: true })
      .click();
    await page.locator(".modal-overlay").waitFor({ state: "hidden" });
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Origin " + stamp })
      .click();
    const row = page.locator("#message-" + source.id);
    await row.waitFor();
    await row.hover();
    await row.getByRole("button", { name: "管理", exact: true }).click();
    await row.getByRole("button", { name: "转发", exact: true }).click();
    await page
      .locator(".modal .contact-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    await Promise.race([
      commit,
      new Promise((_, reject) =>
        setTimeout(() => reject(Error("Forward commit timeout")), 15000),
      ),
    ]);
    assert.equal(records.length, 1);
    for (const key of [
      "text",
      "attachmentId",
      "replyToId",
      "mentionAll",
      "mentionedUserIds",
    ])
      assert.equal(Object.hasOwn(records[0], key), false);
    assert.equal(records[0].forwardSourceVersion, source.contentVersion);
    assert.ok(records[0].forwardAdmission?.startsWith("fa1."));
    assert.ok(records[0].sendAdmission?.startsWith("sa1."));
    await page.reload();
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    await page.getByRole("button", { name: "待发送", exact: true }).click();
    await page.waitForFunction(
      () => !document.querySelector(".message-meta button"),
    );
    assert.equal(
      records.length,
      1,
      "a committed original must be queried, not sent again",
    );
    assert.equal(lookups.length, 1);
    assert.deepEqual(lookups[0], records[0]);
    faultMode = "never";
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Origin " + stamp })
      .click();
    const secondSource = page.locator("#message-" + source.id);
    await secondSource.hover();
    await secondSource
      .getByRole("button", { name: "管理", exact: true })
      .click();
    await secondSource
      .getByRole("button", { name: "转发", exact: true })
      .click();
    await page
      .locator(".modal .contact-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    await Promise.race([
      neverArrived,
      new Promise((_, reject) =>
        setTimeout(() => reject(Error("Never-arrived fault timeout")), 15000),
      ),
    ]);
    const notArrived = (
      await req(
        "/conversations/" + destination.id + "/messages?after=0",
        "GET",
        undefined,
        bob.token,
      )
    ).items.filter((message) => message.clientMsgId === records[1].clientMsgId);
    assert.equal(notArrived.length, 0);
    await page.reload();
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    await page.getByRole("button", { name: "待发送", exact: true }).click();
    await page.waitForFunction(
      () => !document.querySelector(".message-meta button"),
    );
    assert.equal(records.length, 3);
    assert.deepEqual(records[2], records[1]);
    assert.deepEqual(lookups[1], records[1]);
    const createdOnce = (
      await req(
        "/conversations/" + destination.id + "/messages?after=0",
        "GET",
        undefined,
        bob.token,
      )
    ).items.filter((message) => message.clientMsgId === records[1].clientMsgId);
    assert.equal(createdOnce.length, 1);
    hold = false;
    const actual = (
      await req(
        "/conversations/" + destination.id + "/messages?after=0",
        "GET",
        undefined,
        bob.token,
      )
    ).items.filter((m) => m.clientMsgId === records[0].clientMsgId);
    assert.equal(actual.length, 1);
    const forwarded = actual[0];
    assert.equal(forwarded.text, source.text);
    assert.equal(forwarded.forwarded, true);
    assert.equal(forwarded.forwardUnavailable, undefined);
    await page
      .locator("#message-" + forwarded.id)
      .getByText(source.text, { exact: true })
      .waitFor();
    await page.locator("#message-" + forwarded.id).hover();
    await page
      .locator("#message-" + forwarded.id)
      .getByRole("button", { name: "管理", exact: true })
      .click();
    assert.equal(
      await page
        .locator("#message-" + forwarded.id)
        .getByRole("button", { name: "编辑", exact: true })
        .count(),
      0,
    );
    await page
      .locator("#message-" + forwarded.id)
      .getByRole("button", { name: "撤回", exact: true })
      .waitFor();
    hold = true;
    await req(
      "/conversations/" + origin.id,
      "PATCH",
      { ownerId: bob.user.id },
      a.token,
    );
    const members = (
      await req(
        "/conversations/" + origin.id + "/members",
        "GET",
        undefined,
        bob.token,
      )
    ).items;
    const member = members.find((m) => m.id === a.user.id);
    assert.ok(member);
    await req(
      "/conversations/" +
        origin.id +
        "/members/" +
        a.user.id +
        "?membershipGeneration=" +
        member.membershipGeneration,
      "DELETE",
      undefined,
      bob.token,
    );
    await page
      .locator("#message-" + forwarded.id)
      .getByText("内容暂不可用", { exact: true })
      .waitFor({ timeout: 15000 });
    assert.equal(
      await page
        .locator("#message-" + forwarded.id)
        .getByText(source.text, { exact: true })
        .count(),
      0,
    );
    const cached = await page.evaluate(async (id) => {
      const db = await new Promise((resolve, reject) => {
        const r = indexedDB.open("JobsIM", 1);
        r.onsuccess = () => resolve(r.result);
        r.onerror = () => reject(r.error);
      });
      const tx = db.transaction("spaces");
      const values = await new Promise((resolve) => {
        const r = tx.objectStore("spaces").getAll();
        r.onsuccess = () => resolve(r.result);
      });
      db.close();
      return values
        .filter((v) => v && v.messages)
        .flatMap((v) => Object.values(v.messages).flat())
        .filter((m) => m.id === id);
    }, forwarded.id);
    assert.ok(cached.length);
    assert.ok(
      cached.every(
        (m) =>
          !m.text && !m.attachmentId && !m.attachmentURL && !m.forwardAdmission,
      ),
    );
    hold = false;
    const raceGroup = await req(
      "/conversations",
      "POST",
      {
        kind: "group",
        title: "Forward Race " + stamp,
        memberIds: [bob.user.id],
        requestId: crypto.randomUUID(),
      },
      a.token,
    );
    raceSource = await req(
      "/conversations/" + raceGroup.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Late response private body " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Race " + stamp })
      .click();
    const raceRow = page.locator("#message-" + raceSource.id);
    await raceRow.waitFor();
    await raceRow.hover();
    await raceRow.getByRole("button", { name: "管理", exact: true }).click();
    await raceRow.getByRole("button", { name: "转发", exact: true }).click();
    faultMode = "race";
    const observedLateResponse = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/conversations\/[^/]+\/messages$/.test(
          new URL(response.url()).pathname,
        ),
    );
    await page
      .locator(".modal .contact-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    const lateAccepted = await Promise.race([
      lateRace,
      new Promise((_, reject) =>
        setTimeout(() => reject(Error("Late response race timeout")), 15000),
      ),
    ]);
    assert.equal((await observedLateResponse).status(), 200);
    await page.waitForTimeout(150);
    const lateRow = page.locator("#message-" + lateAccepted.id);
    await lateRow.getByText("内容暂不可用", { exact: true }).waitFor();
    assert.equal(
      await lateRow.getByText(raceSource.text, { exact: true }).count(),
      0,
      "late original POST revived an invalidated projection",
    );
    const softSource = await req(
      "/conversations/" + raceGroup.id + "/messages",
      "POST",
      {
        clientMsgId: crypto.randomUUID(),
        type: "text",
        text: "Recoverable projection " + stamp,
        restoreEpoch: "1",
      },
      bob.token,
    );
    await page
      .locator(".conversation-row")
      .filter({ hasText: "Forward Race " + stamp })
      .click();
    const softSourceRow = page.locator("#message-" + softSource.id);
    await softSourceRow.waitFor();
    await softSourceRow.hover();
    await softSourceRow
      .getByRole("button", { name: "管理", exact: true })
      .click();
    await softSourceRow
      .getByRole("button", { name: "转发", exact: true })
      .click();
    faultMode = "plain";
    const softCommitted = page.waitForResponse(
      (response) =>
        response.request().method() === "POST" &&
        /\/api\/conversations\/[^/]+\/messages$/.test(
          new URL(response.url()).pathname,
        ) &&
        response.request().postDataJSON()?.forwardOfId === softSource.id &&
        response.status() === 200,
    );
    await page
      .locator(".modal .contact-row")
      .filter({ hasText: "Forward Peer " + stamp })
      .click();
    const softResult = await (await softCommitted).json();
    const softPage = await req(
      "/conversations/" + destination.id + "/messages?after=0",
      "GET",
      undefined,
      bob.token,
    );
    const soft = softPage.items.find((message) => message.id === softResult.id);
    assert.ok(soft);
    const softRow = page.locator("#message-" + soft.id);
    await softRow.getByText(softSource.text, { exact: true }).waitFor();
    for (const failureStatus of [503, 200]) {
      const unavailableRead = (route) =>
        route.fulfill({
          status: failureStatus,
          contentType: "application/json",
          body: JSON.stringify(
            failureStatus === 503
              ? { error: { code: "FORWARD_QUALIFICATION_UNAVAILABLE" } }
              : {
                  ...soft,
                  text: "",
                  attachmentId: undefined,
                  attachmentURL: undefined,
                  forwardAdmission: undefined,
                  forwardUnavailable: true,
                },
          ),
        });
      await page.route("**/api/messages/" + soft.id, unavailableRead);
      await softRow
        .getByText("内容暂不可用", { exact: true })
        .waitFor({ timeout: 12000 });
      const replayHistory = (route) =>
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({ items: [soft], cursor: softPage.cursor }),
        });
      const replayPattern =
        "**/api/conversations/" + destination.id + "/messages?*";
      await page.route(replayPattern, replayHistory);
      await page.waitForTimeout(3500);
      assert.equal(
        await softRow.getByText(softSource.text, { exact: true }).count(),
        0,
        "same-version late history revived unknown source content after status " +
          failureStatus,
      );
      await softRow.getByText("内容暂不可用", { exact: true }).waitFor();
      await page.unroute(replayPattern, replayHistory);
      await page.unroute("**/api/messages/" + soft.id, unavailableRead);
      await softRow
        .getByText(softSource.text, { exact: true })
        .waitFor({ timeout: 12000 });
    }
    const out = path.join(__dirname, "..", "报告");
    fs.mkdirSync(out, { recursive: true });
    await page.screenshot({
      path: path.join(out, "Web-Forward-Source-Unavailable.png"),
    });
    assert.deepEqual(errors, []);
    fs.writeFileSync(
      path.join(out, "forward-result.json"),
      JSON.stringify(
        {
          status: "passed",
          staticDelivery:
            "actual built Dist supplied by isolated Playwright route at API HTTPS origin; deployment static delivery tested separately",
          at: new Date().toISOString(),
          checks: [
            "current exact source/content version and destination admission",
            "reference-only durable Outbox and original proof",
            "lost response + browser reload queries original result without duplicate POST",
            "never-arrived POST remains absent; explicit retry first looks up 404 then creates exactly once with identical original body",
            "late successful original POST cannot roll back a newer invalidation already obtained by sync",
            "temporary 503 or 200 unavailable exact-read masks the projection; replayed real same-version history cannot reopen it, only a later successful exact GET can",
            "own forward hides edit and keeps recall",
            "source group removal clears target projection and persistent cache without a new message event",
          ],
          errors,
        },
        null,
        2,
      ),
    );
    console.log("PASS Web reference forwarding against real HTTPS/PostgreSQL");
  } catch (error) {
    console.error("Notices", await page.getByRole("status").allTextContents());
    throw error;
  } finally {
    await browser.close();
    await req("/auth/logout", "POST", {}, a.token).catch(() => {});
    await req("/auth/logout", "POST", {}, bob.token).catch(() => {});
  }
})().catch((e) => {
  console.error(e.stack ?? String(e));
  process.exitCode = 1;
});
