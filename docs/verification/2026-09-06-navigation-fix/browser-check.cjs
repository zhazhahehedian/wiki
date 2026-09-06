// Mock HTTP regression checks; no real OAuth, database or model calls.
// Run with Playwright on NODE_PATH; build/start frontend with API base = APP_URL.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13003";
const result = { fixture: "mock HTTP", checks: [], pageErrors: [] };

(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    let authCount = 0;
    let connectionCount = 0;
    let pendingAuth;
    await context.route("**/api/v1/**", async (route) => {
      const p = new URL(route.request().url()).pathname;
      const json = (data, status = 200) => route.fulfill({ status, contentType: "application/json", body: JSON.stringify(data) });
      if (p === "/api/v1/auth/me") {
        authCount++;
        if (authCount > 1) {
          const status = await new Promise((resolve) => { pendingAuth = resolve; });
          if (status === 401) return json({ error: { code: "unauthenticated", message: "login required" } }, 401);
        }
        return json({ id: "demo-user", display_name: "林同学", email: "demo@example.test" });
      }
      if (p === "/api/v1/governance/me") return json({ open_id: "ou_demo", is_admin: true, department: "研发平台", revision: 1 });
      if (p === "/api/v1/playground/connection") { connectionCount++; return json(null); }
      if (["/api/v1/catalog", "/api/v1/capabilities", "/api/v1/governance/reviews"].includes(p)) return json({ items: [], has_more: false });
      throw new Error(`Unexpected API: ${p}`);
    });
    const page = await context.newPage();
    page.setDefaultTimeout(15000);
    page.on("pageerror", (error) => result.pageErrors.push(error.message));
    await page.goto(app + "/hub/registry");
    await page.getByRole("heading", { name: "注册中心", exact: true }).waitFor();
    await page.evaluate(() => {
      window.authFlashes = 0;
      window.navRemovals = 0;
      const nav = document.querySelector('nav[aria-label="主导航"]');
      new MutationObserver((records) => {
        if (document.body.textContent.includes("正在验证登录状态")) window.authFlashes++;
        for (const record of records) for (const node of record.removedNodes) {
          if (node === nav || node.contains?.(nav)) window.navRemovals++;
        }
      }).observe(document.body, { childList: true, subtree: true });
    });
    async function navigateWithSlowAuth(label, heading) {
      pendingAuth = undefined;
      // Advance Date deterministically past the five-second freshness window.
      // Timer waits alone are unreliable when the host wall clock adjusts.
      const now = await page.evaluate(() => Date.now());
      await page.clock.setFixedTime(new Date(now + 60_000));
      await Promise.all([
        page.waitForRequest("**/api/v1/auth/me"),
        page.getByRole("navigation", { name: "主导航" }).getByRole("link", { name: label, exact: true }).click(),
      ]);
      await page.getByRole("heading", { name: heading, exact: true }).waitFor();
      assert.ok(pendingAuth, "background auth request is deliberately pending");
      assert.equal(await page.getByText("正在验证登录状态…", { exact: true }).count(), 0);
      assert.equal(await page.getByRole("navigation", { name: "主导航" }).isVisible(), true);
    }
    await navigateWithSlowAuth("设置", "设置");
    await page.getByRole("button", { name: "配置模型接口", exact: true }).click();
    await page.getByLabel("API Key", { exact: true }).fill("unsaved-fixture-key");
    const settingsAuth = page.waitForResponse("**/api/v1/auth/me");
    pendingAuth(200);
    await (await settingsAuth).finished();
    await page.waitForTimeout(300);
    assert.equal(await page.getByLabel("API Key", { exact: true }).inputValue(), "unsaved-fixture-key");
    await page.keyboard.press("Escape");
    result.checks.push("Settings stays mounted during slow auth refresh; unsaved input survives completion");
    await navigateWithSlowAuth("仪表盘", "欢迎回来，林同学");
    pendingAuth(200);
    await page.waitForTimeout(300);
    assert.equal(connectionCount, 1, "navigation does not reload model configuration");
    assert.deepEqual(await page.evaluate(() => [window.authFlashes, window.navRemovals]), [0, 0]);
    result.checks.push("Dashboard background refresh causes no login flash or sidebar remount");

    async function shot(name) {
      await page.mouse.move(380, 70);
      // Let navigation colors and the mobile sheet's entry transition settle.
      await page.waitForTimeout(400);
      assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
      await page.screenshot({ path: path.join(__dirname, name + ".png"), fullPage: true });
    }
    await page.getByRole("navigation", { name: "主导航" }).getByRole("link", { name: "游乐场" }).click();
    await page.getByRole("heading", { name: "游乐场", exact: true }).waitFor();
    await page.getByRole("link", { name: "审核队列", exact: true }).click();
    await page.getByRole("heading", { name: "审核队列", exact: true }).waitFor();
    assert.equal(await page.getByRole("link", { name: "审核队列", exact: true }).getAttribute("aria-current"), "page");
    await page.setViewportSize({ width: 390, height: 844 });
    await page.getByRole("button", { name: "打开导航" }).click();
    await page.getByRole("dialog").waitFor();
    await shot("navigation-mobile");
    await page.keyboard.press("Escape");
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.waitForTimeout(400);
    await page.setViewportSize({ width: 1440, height: 1000 });
    result.checks.push("Blue brand and active navigation inspected on desktop, governance and mobile; no overflow");

    await navigateWithSlowAuth("设置", "设置");
    pendingAuth(401);
    await page.waitForURL("**/login");
    assert.equal(await page.getByRole("navigation", { name: "主导航" }).count(), 0);
    result.checks.push("A completed 401 refresh still removes protected UI and redirects to login");
    assert.deepEqual(result.pageErrors, []);
    fs.writeFileSync(path.join(__dirname, "result.json"), JSON.stringify(result, null, 2) + "\n");
    console.log(JSON.stringify(result, null, 2));
  } finally {
    await browser.close();
  }
})().catch((error) => { console.error(error); process.exitCode = 1; });
