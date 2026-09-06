// Production-build route checks with mock HTTP; no real accounts or services.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13003";
const result = { fixture: "mock HTTP", checks: [], errors: [] };
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    let releaseCatalog;
    const catalogGate = new Promise((resolve) => { releaseCatalog = resolve; });
    await context.route("**/api/v1/**", async (route) => {
      const p = new URL(route.request().url()).pathname;
      let data;
      if (p === "/api/v1/auth/me") data = { id: "fixture", display_name: "林同学" };
      else if (p === "/api/v1/governance/me") data = { open_id: "ou_fixture", is_admin: true, department: "研发平台", revision: 1 };
      else if (p === "/api/v1/playground/connection") data = null;
      else if (["/api/v1/catalog", "/api/v1/capabilities", "/api/v1/governance/reviews", "/api/v1/governance/audit", "/api/v1/governance/profiles"].includes(p)) {
        if (p === "/api/v1/catalog") await catalogGate;
        data = { items: [], has_more: false };
      } else throw new Error(`Unexpected API ${p}`);
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(data) });
    });
    let releaseChunk;
    const chunkGate = new Promise((resolve) => { releaseChunk = resolve; });
    await context.route("**/_next/static/chunks/app/hub/registry/page-*.js", async (route) => {
      await chunkGate;
      await route.continue();
    });
    const page = await context.newPage();
    page.on("pageerror", (error) => result.errors.push(error.message));
    page.setDefaultTimeout(15000);
    await page.goto(app + "/hub");
    await page.getByRole("heading", { name: "欢迎回来，林同学" }).waitFor();
    await page.getByRole("link", { name: "注册中心", exact: true }).click();
    await page.getByRole("status", { name: "正在加载页面", exact: true }).waitFor();
    assert.equal(await page.getByRole("navigation", { name: "主导航" }).isVisible(), true);
    await page.mouse.move(1400, 70);
    await page.waitForTimeout(400);
    await page.screenshot({ path: path.join(__dirname, "route-loading.png") });
    result.checks.push("Delayed registry chunk shows a route loading state while navigation remains visible");
    releaseChunk();
    await page.getByRole("heading", { name: "注册中心", exact: true }).waitFor();
    await page.getByText("正在加载能力…", { exact: true }).waitFor();
    result.checks.push("Registry shell renders before its delayed catalog response");
    releaseCatalog();
    await page.getByRole("heading", { name: "从第一个能力开始", exact: true }).waitFor();
    for (const label of ["我的发布", "设置", "游乐场", "审核队列", "审计日志", "部门授权"]) {
      await page.getByRole("link", { name: label, exact: true }).click();
      await page.getByRole("heading", { name: label, exact: true }).waitFor();
    }
    result.checks.push("All seven independent menu routes render after client navigation");
    for (const route of ["registry", "publications", "settings", "playground", "reviews", "audit", "profiles"]) {
      const response = await context.request.get(app + "/hub/" + route);
      assert.equal(response.status(), 200, route);
    }
    assert.equal((await context.request.get(app + "/hub/unknown-fixture")).status(), 404);
    assert.deepEqual(result.errors, []);
    fs.writeFileSync(path.join(__dirname, "result.json"), JSON.stringify(result, null, 2) + "\n");
    console.log(JSON.stringify(result, null, 2));
  } finally { await browser.close(); }
})().catch((error) => { console.error(error); process.exitCode = 1; });
