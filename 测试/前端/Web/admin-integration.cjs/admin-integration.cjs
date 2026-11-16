const reportRun = String(Date.now()) + "-" + process.pid;
const fs = require("node:fs"),
  path = require("node:path"),
  https = require("node:https"),
  crypto = require("node:crypto"),
  assert = require("node:assert/strict");
const { chromium } = require(
  process.env.JOBSIM_PLAYWRIGHT_MODULE || "playwright",
);
const server = process.env.JOBSIM_SERVER_URL || "https://127.0.0.1:18443",
  fixturePath = process.env.JOBSIM_ADMIN_FIXTURE,
  caPath = process.env.JOBSIM_CA_FILE;
if (!fixturePath || !caPath)
  throw Error("Provide a private test admin fixture and CA");
if ((fs.statSync(fixturePath).mode & 0o77) !== 0)
  throw Error("Admin fixture must be private");
let fixture;
try {
  fixture = JSON.parse(fs.readFileSync(fixturePath, "utf8"));
} catch {
  throw Error("Invalid private fixture");
}
if ((fs.statSync(fixture.secretFile).mode & 0o77) !== 0)
  throw Error("Enrollment file must be private");
const secret = fs.readFileSync(fixture.secretFile, "utf8").trim(),
  agent = new https.Agent({ ca: fs.readFileSync(caPath) });
function decode32(text) {
  let bits = 0,
    count = 0;
  const bytes = [];
  for (const char of text.toUpperCase()) {
    const value = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567".indexOf(char);
    if (value < 0) throw Error("Invalid enrollment");
    bits = (bits << 5) | value;
    count += 5;
    if (count >= 8) {
      count -= 8;
      bytes.push((bits >> count) & 255);
      bits &= (1 << count) - 1;
    }
  }
  return Buffer.from(bytes);
}
function otp(step) {
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));
  const digest = crypto
      .createHmac("sha1", decode32(secret))
      .update(counter)
      .digest(),
    offset = digest[digest.length - 1] & 15;
  return String((digest.readUInt32BE(offset) & 0x7fffffff) % 1000000).padStart(
    6,
    "0",
  );
}
let lastStep = Number(process.env.JOBSIM_ADMIN_AFTER_STEP || 0);
async function freshCode() {
  while (Math.floor(Date.now() / 30000) <= lastStep)
    await new Promise((resolve) =>
      setTimeout(resolve, Math.min(1000, 30000 - (Date.now() % 30000) + 100)),
    );
  lastStep = Math.floor(Date.now() / 30000);
  return otp(lastStep);
}
async function verifiedPin() {
  return new Promise((resolve, reject) => {
    https
      .get(server + "/api/bootstrap", { agent }, (response) => {
        const cert = response.socket.getPeerCertificate().raw;
        response.resume();
        response.on("end", () =>
          response.statusCode === 200
            ? resolve(
                crypto
                  .createHash("sha256")
                  .update(
                    new crypto.X509Certificate(cert).publicKey.export({
                      type: "spki",
                      format: "der",
                    }),
                  )
                  .digest("base64"),
              )
            : reject(Error("Safety server unavailable")),
        );
      })
      .on("error", (error) => reject(Error(error.code ?? "TLS")));
  });
}
async function forward(request) {
  assert.equal(new URL(request.url()).origin, new URL(server).origin);
  return new Promise((resolve, reject) => {
    const outgoing = https.request(
      request.url(),
      { method: request.method(), headers: request.headers(), agent },
      (response) => {
        response.resume();
        response.on("end", () => resolve(response.statusCode));
      },
    );
    outgoing.on("error", (error) => reject(Error(error.code ?? "NETWORK")));
    outgoing.end(request.postData() ?? undefined);
  });
}
async function nativeAPI(route, method = "GET", body, token) {
  return new Promise((resolve, reject) => {
    const request = https.request(
      server + "/api" + route,
      {
        agent,
        method,
        headers: {
          "Content-Type": "application/json",
          ...(token ? { Authorization: "Bearer " + token } : {}),
        },
      },
      (response) => {
        let raw = "";
        response.on("data", (chunk) => (raw += chunk));
        response.on("end", () => {
          try {
            const data = JSON.parse(raw || "{}");
            response.statusCode < 400
              ? resolve(data)
              : reject(
                  Error("HTTP " + response.statusCode + " " + data.error?.code),
                );
          } catch {
            reject(Error("Invalid test API response"));
          }
        });
      },
    );
    request.on("error", (error) => reject(Error(error.code ?? "NETWORK")));
    request.setTimeout(8000, () => request.destroy(Error("Test timeout")));
    request.end(body === undefined ? undefined : JSON.stringify(body));
  });
}
(async () => {
  const pin = await verifiedPin();
  const browser = await chromium.launch({
    headless: true,
    executablePath:
      process.env.BROWSER_EXECUTABLE ||
      "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
    args: ["--ignore-certificate-errors-spki-list=" + pin],
  });
  const page = await browser.newPage({
      viewport: { width: 1440, height: 1060 },
    }),
    errors = [];
  page.on("pageerror", (error) => errors.push(error.message));
  page.on("response", async (response) => {
    if (
      new URL(response.url()).pathname.startsWith("/api/") &&
      response.status() >= 400
    ) {
      const body = await response.json().catch(() => ({}));
      console.log(
        new URL(response.url()).pathname,
        response.status(),
        body.error?.code ?? "NO_CODE",
      );
    }
  });
  const actors = [];
  let testReport;
  try {
    const stamp = Date.now().toString(36),
      password = "Private report test " + stamp + " phrase";
    for (const role of ["reporter", "target"])
      actors.push(
        await nativeAPI("/auth/register", "POST", {
          username: role + "_" + stamp,
          password,
          requestId: crypto.randomUUID(),
          deviceName: "Admin report test peer",
        }),
      );
    testReport = await nativeAPI(
      "/reports",
      "POST",
      {
        targetType: "user",
        targetId: actors[1].user.id,
        reason: "Admin UI report " + stamp,
        requestId: crypto.randomUUID(),
      },
      actors[0].token,
    );
    await page.goto(server);
    await page.getByRole("button", { name: "登录", exact: true }).click();
    await page.locator("[name=username]").fill(fixture.username);
    await page.locator("[name=password]").fill(fixture.password);
    console.log(
      "Auth form validity",
      await page.locator(".modal form").evaluate((form) => ({
        valid: form.checkValidity(),
        invalid: [...form.querySelectorAll(":invalid")].map((input) => ({
          name: input.name,
          pattern: input.validity.patternMismatch,
          short: input.validity.tooShort,
          long: input.validity.tooLong,
          missing: input.validity.valueMissing,
        })),
      })),
    );
    await page
      .locator(".modal form")
      .getByRole("button", { name: "登录", exact: true })
      .click();
    await page.locator("[name=otp]").waitFor();
    await page.locator("[name=otp]").fill(await freshCode());
    await page
      .locator(".modal form")
      .getByRole("button", { name: "登录", exact: true })
      .click();
    await page.locator(".modal-overlay").waitFor({ state: "hidden" });
    console.log("PASS browser mandatory MFA login");
    await page.getByRole("button", { name: "设置", exact: true }).click();
    await page.getByRole("button", { name: "管理后台", exact: true }).click();
    await page.locator(".admin-columns").waitFor();
    const readCaps = () =>
      page.evaluate(async () => {
        const response = await fetch("/api/capabilities", {
          credentials: "include",
        });
        if (!response.ok) throw Error("Capability lookup failed");
        return response.json();
      });
    const original = await readCaps();
    const records = [];
    let loseResponse = true;
    await page.route("**/api/capabilities/pins", async (route) => {
      if (route.request().method() !== "PUT") return route.continue();
      records.push(route.request().postDataJSON());
      if (loseResponse) {
        loseResponse = false;
        assert.equal(await forward(route.request()), 200);
        await route.abort("failed");
      } else await route.continue();
    });
    const pinRow = () =>
      page
        .locator(".admin-columns section")
        .nth(1)
        .getByRole("button", { name: /^置顶/ });
    await pinRow().click();
    await page.locator(".admin-operation [name=otp]").fill(await freshCode());
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "提交", exact: true })
      .click();
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "重试", exact: true })
      .waitFor();
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "重试", exact: true })
      .click();
    await page.locator(".admin-operation").waitFor({ state: "hidden" });
    assert.equal(records.length, 2);
    assert.equal(records[0].requestId, records[1].requestId);
    assert.equal(records[0].stepUpId, records[1].stepUpId);
    const changed = await readCaps();
    assert.equal(changed.features.pins, !original.features.pins);
    assert.equal(
      BigInt(changed.versions.pins),
      BigInt(original.versions.pins ?? "0") + 1n,
    );
    console.log("PASS bound step-up and original-result retry");
    await page.unroute("**/api/capabilities/pins");
    await pinRow().click();
    await page.locator(".admin-operation [name=otp]").fill(await freshCode());
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "提交", exact: true })
      .click();
    await page.locator(".admin-operation").waitFor({ state: "hidden" });
    const restored = await readCaps();
    assert.equal(restored.features.pins, original.features.pins);
    assert.equal(
      BigInt(restored.versions.pins),
      BigInt(changed.versions.pins) + 1n,
    );
    const reportWrites = [];
    let loseReport = true;
    await page.route("**/api/admin/reports/" + testReport.id, async (route) => {
      if (route.request().method() !== "PATCH") return route.continue();
      reportWrites.push(route.request().postDataJSON());
      if (loseReport) {
        loseReport = false;
        assert.equal(await forward(route.request()), 200);
        await route.abort("failed");
      } else await route.continue();
    });
    await page
      .locator(".admin-reports")
      .getByRole("button", { name: new RegExp(testReport.reason) })
      .click();
    await page.locator(".admin-operation select").selectOption("reviewed");
    await page
      .locator(".admin-operation textarea[name=reason]")
      .fill("Reviewed in the real administrator interface");
    await page.locator(".admin-operation [name=otp]").fill(await freshCode());
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "提交", exact: true })
      .click();
    await page
      .locator(".admin-operation")
      .getByRole("button", { name: "重试", exact: true })
      .click();
    await page.locator(".admin-operation").waitFor({ state: "hidden" });
    assert.equal(reportWrites.length, 2);
    assert.equal(reportWrites[0].requestId, reportWrites[1].requestId);
    assert.equal(reportWrites[0].stepUpId, reportWrites[1].stepUpId);
    const reportResult = await nativeAPI(
      "/reports",
      undefined,
      undefined,
      actors[0].token,
    );
    const changedReport = reportResult.items.find(
      (item) => item.id === testReport.id,
    );
    assert.equal(changedReport.state, "reviewed");
    assert.equal(
      BigInt(changedReport.version),
      BigInt(testReport.version) + 1n,
    );
    await page.unroute("**/api/admin/reports/" + testReport.id);
    assert.deepEqual(errors, []);
    const report = path.join(__dirname, "../../../../项目相关报告/测试/前端/Web/执行记录", reportRun);
    fs.mkdirSync(report, { recursive: true });
    await page.screenshot({ path: path.join(report, "Web-Admin-HTTPS.png") });
    fs.writeFileSync(
      path.join(report, "admin-result.json"),
      JSON.stringify(
        {
          status: "passed",
          at: new Date().toISOString(),
          lastUsedStep: lastStep,
          checks: [
            "CA and hostname verified before browser certificate pin",
            "mandatory administrator MFA login",
            "operation target/payload/version-bound step-up",
            "lost response same request and same authorization original result",
            "capability changed once and restored to original setting",
            "real administrator report resolution with bound MFA; lost response retries identical proof and advances report version once",
          ],
          errors,
        },
        null,
        2,
      ),
    );
    console.log(
      "PASS real browser administrator operations; last used step " + lastStep,
    );
  } finally {
    await page
      .evaluate(async () => {
        try {
          const response = await fetch("/api/auth/csrf", {
            credentials: "include",
          });
          if (response.ok) {
            const { csrfToken } = await response.json();
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
    for (const actor of actors)
      await nativeAPI("/auth/logout", "POST", {}, actor.token).catch(() => {});
    await browser.close();
  }
})().catch((error) => {
  console.error(String(error.message).split("\n")[0]);
  console.error(
    String(error.stack)
      .split("\n")
      .find((line) => line.includes("admin-integration.cjs:")) ?? "",
  );
  process.exitCode = 1;
});
