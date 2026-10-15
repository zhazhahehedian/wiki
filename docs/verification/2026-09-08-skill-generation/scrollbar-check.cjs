// Browser regression for the rotating spinner creating a 1px scroll overflow.
// Requires Playwright on NODE_PATH. All business requests use fixtures.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13004";
const result = {
  fixture: "mock generation; sample 120 animation frames per case",
  checks: [],
  pageErrors: [],
};
(async () => {
  const browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox"],
  });
  try {
    for (const test of [
      { name: "desktop-short", width: 1440, height: 1100 },
      { name: "wide-short", width: 3840, height: 1929 },
      { name: "mobile-short", width: 390, height: 844 },
      { name: "desktop-125-percent", width: 1440, height: 1100, zoom: 1.25 },
      { name: "desktop-long", width: 1440, height: 1100, long: true },
    ]) {
      const page = await browser.newPage({
        viewport: { width: test.width, height: test.height },
      });
      page.on("pageerror", (e) => result.pageErrors.push(e.message));
      await page.route("**/api/v1/**", (route) => {
        const p = new URL(route.request().url()).pathname;
        const data = p.endsWith("/auth/me")
          ? { id: "fixture", display_name: "布局验证" }
          : p.endsWith("/governance/me")
            ? { is_admin: false }
            : p.endsWith("/playground/connection")
              ? {
                  protocol: "openai",
                  baseUrl: "https://relay.example.test/v1",
                  models: ["fixture-model"],
                  defaultModel: "fixture-model",
                  hasKey: true,
                  version: 1,
                }
              : null;
        return route.fulfill({
          contentType: "application/json",
          body: JSON.stringify(data),
        });
      });
      await page.addInitScript(() => {
        const original = window.fetch;
        window.fetch = (url, init) =>
          String(url).endsWith("/skill-builder/turn")
            ? new Promise((resolve, reject) =>
                init.signal.addEventListener("abort", () =>
                  reject(new DOMException("Aborted", "AbortError")),
                ),
              )
            : original(url, init);
      });
      await page.goto(`${app}/hub/registry/create-skill`);
      if (test.zoom)
        await page.evaluate((zoom) => {
          document.documentElement.style.zoom = String(zoom);
        }, test.zoom);
      await page
        .getByLabel("你的需求或修改意见", { exact: true })
        .fill(
          test.long
            ? "请按下面的要求整理输入，输出进展与风险清单。\n".repeat(80)
            : "帮我生成一个合同审查的法务skill",
        );
      await page.getByRole("button", { name: "发送需求", exact: true }).click();
      await page.getByRole("status", { name: "Skill 生成进度" }).waitFor();
      const messages = page.getByLabel("创建对话消息", { exact: true });
      if (test.long)
        await messages.evaluate((el) => {
          el.scrollTop = 150;
        });
      const samples = await messages.evaluate(
        (el) =>
          new Promise((resolve) => {
            const values = [];
            let frames = 0;
            function tick() {
              values.push({
                clientHeight: el.clientHeight,
                scrollHeight: el.scrollHeight,
                clientWidth: el.clientWidth,
                scrollTop: el.scrollTop,
                overflow: el.scrollHeight > el.clientHeight,
              });
              if (++frames < 120) requestAnimationFrame(tick);
              else
                resolve([
                  ...new Map(
                    values.map((value) => [JSON.stringify(value), value]),
                  ).values(),
                ]);
            }
            requestAnimationFrame(tick);
          }),
      );
      assert.equal(
        samples.length,
        1,
        `${test.name}: scrollbar geometry changes during animation ${JSON.stringify(samples)}`,
      );
      assert.equal(
        samples[0].overflow,
        !!test.long,
        `${test.name}: short messages must not scroll; long messages must scroll`,
      );
      if (test.long)
        assert.equal(
          samples[0].scrollTop,
          150,
          "animation must not move the reader",
        );
      const animated = await messages
        .locator("svg")
        .evaluate((el) => getComputedStyle(el).animationName);
      assert.notEqual(animated, "none", "keep the loading animation");
      if (test.name === "desktop-short")
        await page.screenshot({
          path: path.join(__dirname, "desktop-scrollbar-fixed.png"),
          fullPage: true,
        });
      if (test.name === "mobile-short")
        await page.screenshot({
          path: path.join(__dirname, "mobile-scrollbar-fixed.png"),
          fullPage: true,
        });
      result.checks.push({ name: test.name, samples, animation: animated });
      await page.getByRole("button", { name: "停止生成", exact: true }).click();
      await page
        .getByRole("status", { name: "Skill 生成进度" })
        .waitFor({ state: "detached" });
      await page.close();
    }
    assert.deepEqual(result.pageErrors, []);
    fs.writeFileSync(
      path.join(__dirname, "scrollbar-result.json"),
      JSON.stringify(result, null, 2) + "\n",
    );
    console.log(JSON.stringify(result, null, 2));
  } finally {
    await browser.close();
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
