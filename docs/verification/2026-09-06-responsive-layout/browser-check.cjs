// Mock HTTP fixtures; run with Playwright on NODE_PATH and a frontend whose
// NEXT_PUBLIC_API_BASE_URL matches APP_URL (default http://localhost:13003).
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13003";
const report = { fixture: "mock HTTP", widths: [], errors: [] };
const items = Array.from({ length: 18 }, (_, i) => ({
  id: `fixture-${i}`, slug: `capability-${i}`, name: `团队能力 ${i + 1}`,
  type: i % 2 ? "skill" : "mcp", status: "published", is_live: true,
  description: "用于检查不同窗口宽度下的卡片排列、文字换行与操作区域。",
  department: "研发平台", visibility: "org", revision: 1,
  owner_open_id: "ou_fixture", created_at: "2026-09-06T08:00:00Z", updated_at: "2026-09-06T08:00:00Z",
}));
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({ viewport: { width: 1440, height: 1000 } });
    await context.route("**/api/v1/**", (route) => {
      const p = new URL(route.request().url()).pathname;
      let data;
      if (p === "/api/v1/auth/me") data = { id: "fixture", display_name: "林同学" };
      else if (p === "/api/v1/governance/me") data = { open_id: "ou_fixture", is_admin: false, department: "研发平台", revision: 1 };
      else if (p === "/api/v1/playground/connection") data = null;
      else if (p === "/api/v1/catalog") data = { items, has_more: false };
      else if (p === "/api/v1/capabilities") data = { items: [], has_more: false };
      else throw new Error(`Unexpected API ${p}`);
      return route.fulfill({ status: 200, contentType: "application/json", body: JSON.stringify(data) });
    });
    const page = await context.newPage();
    page.on("pageerror", (error) => report.errors.push(error.message));
    await page.goto(app + "/hub/registry");
    await page.getByRole("heading", { name: "团队能力 1", exact: true }).waitFor();
    for (const width of [390, 768, 1024, 1440, 1920, 2560, 3840]) {
      await page.setViewportSize({ width, height: 1000 });
      await page.waitForTimeout(200);
      const dimensions = await page.evaluate(() => {
        const main = document.querySelector("#hub-content").getBoundingClientRect();
        const cards = [...document.querySelectorAll('#hub-content a[href^="/hub/registry/capability-"]')].map((el) => el.getBoundingClientRect());
        return { width: innerWidth, main: main.width, columns: cards.filter((r) => Math.abs(r.top - cards[0].top) < 1).length,
          cardWidth: cards[0].width, overflow: document.documentElement.scrollWidth > innerWidth };
      });
      assert.equal(dimensions.main, width >= 1024 ? width - 220 : width);
      assert.equal(dimensions.overflow, false);
      assert.ok(dimensions.cardWidth >= 280);
      report.widths.push(dimensions);
      if (width === 2560) await page.screenshot({ path: path.join(__dirname, `registry-${width}.png`) });
    }
    assert.equal(report.widths[0].columns, 1);
    assert.ok(report.widths[6].columns > report.widths[3].columns);
    await page.setViewportSize({ width: 2560, height: 1200 });
    await page.getByRole("link", { name: "我的发布", exact: true }).click();
    await page.getByRole("heading", { name: "从第一个能力开始", exact: true }).waitFor();
    await page.mouse.move(2400, 70);
    await page.waitForTimeout(300);
    await page.screenshot({ path: path.join(__dirname, "publications-empty-2560.png") });
    await page.setViewportSize({ width: 390, height: 844 });
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
    await page.screenshot({ path: path.join(__dirname, "publications-mobile.png"), fullPage: true });
    assert.deepEqual(report.errors, []);
    fs.writeFileSync(path.join(__dirname, "result.json"), JSON.stringify(report, null, 2) + "\n");
    console.log(JSON.stringify(report, null, 2));
  } finally { await browser.close(); }
})().catch((error) => { console.error(error); process.exitCode = 1; });
