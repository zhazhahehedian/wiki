// Mock business APIs only. Build the frontend with API base http://localhost:13004.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13004";
const draft = {
  slug: "weekly-report",
  name: "团队周报",
  description: "整理每周工作进展",
  instructions: "# 周报\n\n列出进展、风险和下一步。",
  files: [],
};
const result = {
  fixture: "mock model and API; not real model acceptance",
  checks: [],
  pageErrors: [],
};
(async () => {
  const browser = await chromium.launch({
    headless: true,
    args: ["--no-sandbox"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1100 },
    });
    page.on("pageerror", (e) => result.pageErrors.push(e.message));
    await page.clock.install();
    await page.route("**/api/v1/**", (route) => {
      const p = new URL(route.request().url()).pathname;
      const data =
        p === "/api/v1/auth/me"
          ? { id: "fixture", display_name: "林同学" }
          : p === "/api/v1/governance/me"
            ? {
                open_id: "ou_fixture",
                is_admin: false,
                department: "",
                revision: 0,
              }
            : p === "/api/v1/playground/connection"
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
      window.fetch = (url, init) => {
        if (!String(url).endsWith("/api/v1/skill-builder/turn"))
          return original(url, init);
        window.skillRequestCount = (window.skillRequestCount || 0) + 1;
        window.skillRequests = [...(window.skillRequests || []), JSON.parse(init.body)];
        return new Promise((resolve, reject) => {
          window.replyToSkill = (status, body) =>
            resolve(
              new Response(JSON.stringify(body), {
                status,
                headers: { "Content-Type": "application/json" },
              }),
            );
          init.signal.addEventListener(
            "abort",
            () => reject(new DOMException("Aborted", "AbortError")),
            { once: true },
          );
        });
      };
    });
    const progress = page.getByRole("status", { name: "Skill 生成进度" });
    const input = page.getByLabel("你的需求或修改意见", { exact: true });
    await page.goto(`${app}/hub/registry/create-skill`);
    const budget = page.getByLabel("最大输出 Tokens", { exact: true });
    const settings = page.locator("summary").filter({ hasText: "生成设置" });
    assert.equal(await budget.inputValue(), "65536");
    await settings.click();
    await page.screenshot({ path: path.join(__dirname, "desktop-output-settings.png"), fullPage: true });
    await input.fill("根据团队规范整理周报，包括进展、风险和下一步。");
    await page.getByRole("button", { name: "发送需求", exact: true }).click();
    await progress.waitFor();
    assert.equal(await page.evaluate(() => window.skillRequests[0].max_tokens), 65536);
    assert(await budget.isDisabled());
    assert.equal(
      await page
        .getByRole("heading", { name: "从需求到一份可用的 Skill" })
        .count(),
      0,
    );
    assert(
      await progress
        .getByText("正在准备 Skill 草案", { exact: true })
        .isVisible(),
    );
    const spinner = progress.locator("svg").first();
    assert.notEqual(
      await spinner.evaluate((el) => getComputedStyle(el).animationName),
      "none",
    );
    await page.screenshot({
      path: path.join(__dirname, "desktop-generating.png"),
      fullPage: true,
    });
    await page.clock.fastForward(46000);
    assert.match(await progress.innerText(), /已等待 4[6-9] 秒/);
    assert.match(await progress.innerText(), /仍在等待模型完成/);
    await page.screenshot({
      path: path.join(__dirname, "desktop-long-wait.png"),
      fullPage: true,
    });
    await page.evaluate(
      (draft) => window.replyToSkill(200, { message: "已整理草案", draft }),
      draft,
    );
    await page.getByLabel("Skill 名称", { exact: true }).waitFor();
    await progress.waitFor({ state: "detached" });
    assert.equal(await input.inputValue(), "");
    await page
      .getByLabel("SKILL.md 正文", { exact: true })
      .fill("保留我修改的正文。");
    await input.fill("补充负责人和时间节点。");
    await budget.fill("131072");
    await page.getByRole("button", { name: "继续修改", exact: true }).click();
    await progress.waitFor();
    assert.equal(await page.evaluate(() => window.skillRequests[1].max_tokens), 131072);
    assert.match(await progress.innerText(), /正在更新 Skill 草案/);
    assert.equal(
      await page.getByLabel("SKILL.md 正文", { exact: true }).inputValue(),
      "保留我修改的正文。",
    );
    await page.evaluate(() =>
      window.replyToSkill(504, {
        error: {
          code: "model_response_timeout",
          message: "等待模型响应超时，请缩小本次生成范围或稍后重试",
          request_id: "fixture-timeout-id",
        },
      }),
    );
    await page
      .getByRole("alert")
      .filter({ hasText: "等待模型响应超时" })
      .waitFor();
    await progress.waitFor({ state: "detached" });
    assert.equal(await input.inputValue(), "补充负责人和时间节点。");
    assert.equal(
      await page.getByLabel("SKILL.md 正文", { exact: true }).inputValue(),
      "保留我修改的正文。",
    );
    assert(
      await page
        .getByText("请求编号：fixture-timeout-id", { exact: true })
        .isVisible(),
    );
    result.checks.push(
      "Animated pending draft, elapsed time and long wait, valid completion, retained edited draft and input on timeout, safe request id",
    );
    assert.equal(await budget.inputValue(), "131072");
    await budget.fill("1048577");
    await settings.click();
    await page.getByRole("button", { name: "继续修改", exact: true }).click();
    assert.equal(await page.evaluate(() => window.skillRequestCount), 2);
    assert(await budget.isVisible(), "invalid hidden budget must be revealed for correction");
    await budget.fill("262144");
    await page.getByRole("button", { name: "继续修改", exact: true }).click();
    await progress.waitFor();
    assert.equal(await page.evaluate(() => window.skillRequests[2].max_tokens), 262144);
    await page.getByRole("button", { name: "停止生成", exact: true }).click();
    await progress.waitFor({ state: "detached" });
    assert.equal(
      await page.evaluate(() => window.skillRequestCount),
      3,
      "stop must not submit a replacement request",
    );
    assert.equal(await input.inputValue(), "补充负责人和时间节点。");
    await page.setViewportSize({ width: 390, height: 844 });
    await budget.fill("8192");
    await page.screenshot({ path: path.join(__dirname, "mobile-output-settings.png"), fullPage: true });
    await page.getByRole("button", { name: "继续修改", exact: true }).click();
    await progress.waitFor();
    assert(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ),
    );
    await page.screenshot({
      path: path.join(__dirname, "mobile-updating.png"),
      fullPage: true,
    });
    await page.emulateMedia({ reducedMotion: "reduce" });
    assert.equal(
      await spinner.evaluate((el) => getComputedStyle(el).animationName),
      "none",
    );
    await page.getByRole("button", { name: "停止生成", exact: true }).click();
    result.checks.push(
      "Cancel retains work; mobile has no horizontal overflow; reduced motion disables animation",
      "Output budget defaults to 65536; custom 131072/262144 and lower 8192 supported; disabled while pending; invalid collapsed input is revealed without submitting; failure retains budget and edits",
    );
    assert.deepEqual(result.pageErrors, []);
    fs.writeFileSync(
      path.join(__dirname, "result.json"),
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
