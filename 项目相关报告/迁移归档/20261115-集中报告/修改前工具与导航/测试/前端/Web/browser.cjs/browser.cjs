const { chromium } = require(
  process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright",
);
const fs = require("fs"),
  path = require("path"),
  assert = require("node:assert/strict");
(async () => {
  const out = path.join(__dirname, "..", "报告");
  fs.mkdirSync(out, { recursive: true });
  const b = await chromium.launch({
    headless: true,
    executablePath:
      process.env.BROWSER_EXECUTABLE ||
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
  });
  const page = await b.newPage({ viewport: { width: 1440, height: 960 } });
  const errors = [];
  page.on("pageerror", (e) => errors.push(e.message));
  try {
    await page.goto(process.env.JOBSIM_WEB_URL || "http://127.0.0.1:5178");
    await page.getByRole("button", { name: "发送", exact: true }).waitFor();
    await page.evaluate(async () => {
      const store = await import("/Src/storage.ts");
      const { demoSpace } = await import("/Src/demo.ts");
      const key = "browser-test|" + crypto.randomUUID();
      const seed = demoSpace();
      const original = seed.messages[seed.conversations[0].id][0];
      const pending = {
        ...original,
        id: crypto.randomUUID(),
        clientMsgId: crypto.randomUUID(),
        pending: true,
        restoreEpoch: "1",
      };
      await store.queueMessage(key, seed, pending);
      await store.saveView(key, seed);
      const recovered = await store.loadSpace(key);
      if (
        recovered.outbox.length !== 1 ||
        !recovered.messages[pending.conversationId].some(
          (m) => m.clientMsgId === pending.clientMsgId,
        )
      )
        throw Error("A stale view erased the committed Outbox");
      if ((await store.loadSpace(key + "|other-account")).outbox.length)
        throw Error("Account Outbox leaked");
      const reference = {
        ...pending,
        clientMsgId: crypto.randomUUID(),
        type: "forward",
        text: "must not be copied",
        attachmentId: "must-not-be-forwarded",
        forwardOfId: "source-id",
        forwardSourceVersion: "9007199254740997",
        forwardAdmission: "original-source-permit",
        sendAdmission: "original-target-permit",
      };
      const forwardKey = key + "|forward";
      await store.queueMessage(forwardKey, seed, reference);
      const first = await store.prepareOutboxAttempt(
        forwardKey,
        reference.clientMsgId,
      );
      if (!first || first.sendAttempted)
        throw Error("First attempt lost its original state");
      await store.saveView(forwardKey, seed);
      const second = await store.prepareOutboxAttempt(
        forwardKey,
        reference.clientMsgId,
      );
      const { messagePayload, unavailableForward, attachmentPath } =
        await import("/Src/forward.ts");
      if (
        !second?.sendAttempted ||
        JSON.stringify(messagePayload(first)) !==
          JSON.stringify(messagePayload(second))
      )
        throw Error("Retry changed the original forward payload");
      const payload = messagePayload(second);
      if (
        "text" in payload ||
        "attachmentId" in payload ||
        payload.forwardSourceVersion !== "9007199254740997" ||
        payload.forwardAdmission !== "original-source-permit"
      )
        throw Error("Forward copied content or lost original proof");
      const masked = unavailableForward({
        ...reference,
        attachmentURL: "/api/messages/id/attachment",
        attachmentName: "private.jpg",
      });
      if (
        masked.text ||
        masked.attachmentId ||
        masked.attachmentURL ||
        masked.attachmentName ||
        masked.forwardAdmission ||
        !masked.forwardUnavailable
      )
        throw Error("Unavailable projection retained source content");
      const available = {
        ...reference,
        id: "f",
        forwarded: true,
        attachmentURL: "/api/messages/f/attachment",
      };
      if (
        attachmentPath(available) !== "/api/messages/f/attachment" ||
        attachmentPath({ ...available, attachmentURL: "/api/media/source" }) ||
        attachmentPath({
          ...available,
          attachmentURL: "https://other.example/attachment",
        }) ||
        attachmentPath(masked)
      )
        throw Error(
          "Forward media leaked a source ID or accepted a different origin/path",
        );
      await store.freezeOutboxMessage(forwardKey, reference.clientMsgId);
      if (await store.prepareOutboxAttempt(forwardKey, reference.clientMsgId))
        throw Error("Frozen forward was granted a new attempt");
      await store.acknowledgeOutbox(key, ["unrelated-result"]);
      if ((await store.loadSpace(key)).outbox.length !== 1)
        throw Error("Unrelated acknowledgement removed queued work");
      const other = {
        ...pending,
        id: crypto.randomUUID(),
        clientMsgId: crypto.randomUUID(),
        conversationId: "other-conversation",
      };
      await store.queueMessage(key, seed, other);
      await store.freezeConversation(key, pending.conversationId, {
        ...seed,
        conversations: [],
        messages: {},
        outbox: [],
      });
      await store.saveView(key, seed);
      const frozen = await store.loadSpace(key);
      if (
        !frozen.outbox.find((m) => m.clientMsgId === pending.clientMsgId)
          ?.frozen ||
        frozen.outbox.find((m) => m.clientMsgId === other.clientMsgId)?.frozen
      )
        throw Error(
          "Conversation withdrawal failed to isolate its committed queue",
        );
      if (
        frozen.messages[pending.conversationId]?.some(
          (m) => m.clientMsgId === pending.clientMsgId,
        )
      )
        throw Error("A frozen original was projected into history");
      await store.freezeNamespace(
        key,
        { ...seed, restoreEpoch: "2", messages: {} },
        "1",
      );
      await store.saveView(key, { ...seed, restoreEpoch: "2" });
      const restored = await store.loadSpace(key);
      if (
        restored.outbox.length !== 2 ||
        restored.outbox.some(
          (message) => !message.frozen || message.restoreEpoch !== "1",
        )
      )
        throw Error(
          "Restore did not freeze every authoritative queued original",
        );
      if (
        Object.values(restored.messages)
          .flat()
          .some((message) => message.pending)
      )
        throw Error(
          "A stale view resurrected an old pending body after restore",
        );
      await store.saveIntent(key, "report", { requestId: "new-operation" });
      await store.clearIntent(key, "report", "old-operation");
      if (
        (await store.loadIntent(key, "report"))?.requestId !== "new-operation"
      )
        throw Error("An older completion erased a newer operation");
      await store.clearIntent(key, "report", "new-operation");
      await store.acknowledgeOutbox(key, [
        pending.clientMsgId,
        other.clientMsgId,
      ]);
      if ((await store.loadSpace(key)).outbox.length)
        throw Error("Accepted message remains queued");
      const db = await new Promise((resolve, reject) => {
        const r = indexedDB.open("JobsIM", 1);
        r.onsuccess = () => resolve(r.result);
        r.onerror = () => reject(r.error);
      });
      await new Promise((resolve, reject) => {
        const tx = db.transaction("spaces", "readwrite");
        tx.objectStore("spaces").delete(key);
        tx.objectStore("spaces").delete("outbox|" + key);
        tx.oncomplete = resolve;
        tx.onerror = () => reject(tx.error);
      });
      db.close();
    });

    const days = await page.locator(".date-divider").allTextContents();
    assert.ok(
      new Set(days).size >= 2,
      "cross-day history needs distinct date headings",
    );
    assert.equal(await page.locator(".rail [aria-current=page]").count(), 1);
    await page.screenshot({ path: path.join(out, "Web-Light.png") });
    await page
      .getByRole("textbox", { name: "写一条消息…" })
      .fill("浏览器交互验证：保留消息和输入上下文");
    await page.getByRole("button", { name: "发送", exact: true }).click();
    await page
      .locator(".thread-body")
      .getByText("浏览器交互验证：保留消息和输入上下文", { exact: true })
      .waitFor();
    await page.getByRole("button", { name: "设置", exact: true }).click();
    const selects = page.locator(".settings-section select");
    await selects.nth(0).selectOption("en");
    const baseSize = await page
      .locator(".setting-line")
      .first()
      .evaluate((el) => parseFloat(getComputedStyle(el).fontSize));
    await selects.nth(1).selectOption("large");
    assert.ok(
      (await page
        .locator(".setting-line")
        .first()
        .evaluate((el) => parseFloat(getComputedStyle(el).fontSize))) >
        baseSize,
      "large font must visibly scale",
    );
    await selects.nth(1).selectOption("normal");
    await page.getByRole("button", { name: "Dark", exact: true }).click();
    assert.equal(await page.locator("html").getAttribute("data-theme"), "dark");
    await page.screenshot({ path: path.join(out, "Web-Dark-Settings.png") });
    await page.getByRole("button", { name: "System", exact: true }).click();
    await page.emulateMedia({ colorScheme: "dark" });
    await page.waitForFunction(
      () => document.documentElement.dataset.theme === "dark",
    );
    assert.equal(await page.locator("html").getAttribute("data-theme"), "dark");
    await page.screenshot({ path: path.join(out, "Web-System-Settings.png") });
    await page.emulateMedia({ colorScheme: "light" });
    await page.waitForFunction(
      () => document.documentElement.dataset.theme === "light",
    );
    await page.getByRole("button", { name: "Dark", exact: true }).click();
    for (const locale of [
      "zh-CN",
      "zh-TW",
      "bo",
      "en",
      "ja",
      "fr",
      "es",
      "de",
      "ru",
      "ko",
    ]) {
      await selects.nth(0).selectOption(locale);
      assert.equal(await page.locator("html").getAttribute("lang"), locale);
      assert.ok(
        await page.evaluate(
          () => document.documentElement.scrollWidth <= innerWidth,
        ),
        "overflow " + locale,
      );
    }
    await page.setViewportSize({ width: 390, height: 844 });
    await page.screenshot({ path: path.join(out, "Web-Mobile-Settings.png") });
    assert.ok(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    assert.deepEqual(errors, []);
    console.log(
      JSON.stringify({
        status: "passed",
        checks: [
          "demo send",
          "cross-day grouping and current navigation semantics",
          "strict Outbox survives stale view and account isolation",
          "theme",
          "visible font scaling",
          "10 languages",
          "desktop/mobile overflow",
          "no JS errors",
        ],
        screenshots: out,
      }),
    );
    fs.writeFileSync(
      path.join(out, "browser-result.json"),
      JSON.stringify(
        { status: "passed", errors, at: new Date().toISOString() },
        null,
        2,
      ),
    );
  } finally {
    await b.close();
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
