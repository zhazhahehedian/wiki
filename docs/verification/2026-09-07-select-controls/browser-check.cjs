// UI integration fixtures only; no real credentials, backend or model provider.
// Build/start frontend with NEXT_PUBLIC_API_BASE_URL=http://localhost:13003.
// Run with Playwright available on NODE_PATH. APP_URL may override the base URL.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13003";
const longModel =
  "relay/vendor/chat-model-with-a-very-long-identifier-for-mobile-layout-verification";
let connection = null,
  chatPayload,
  discoveryPayload;
const result = {
  fixture: "mock HTTP; not real model acceptance",
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
      viewport: { width: 1440, height: 1000 },
    });
    page.on("pageerror", (e) => result.pageErrors.push(e.message));
    await page.route("**/api/v1/**", async (route) => {
      const request = route.request(),
        p = new URL(request.url()).pathname;
      const json = (data) =>
        route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify(data),
        });
      if (p === "/api/v1/auth/me")
        return json({
          id: "fixture",
          display_name: "林同学",
          email: "demo@example.test",
        });
      if (p === "/api/v1/governance/me")
        return json({
          open_id: "ou_fixture",
          is_admin: true,
          department: "研发平台",
          revision: 1,
        });
      if (p === "/api/v1/playground/connection") {
        if (request.method() === "PUT") {
          const { apiKey, ...input } = request.postDataJSON();
          assert.equal(apiKey, "fixture-only-key");
          connection = { ...input, hasKey: true, version: input.version + 1 };
        }
        return json(connection);
      }
      if (p === "/api/v1/playground/models") {
        discoveryPayload = request.postDataJSON();
        return json({ models: ["relay-chat", longModel] });
      }
      if (p === "/api/v1/playground/chat/stream") {
        chatPayload = request.postDataJSON();
        return route.fulfill({
          contentType: "text/event-stream",
          body: 'event: delta\ndata: {"text":"模拟模型回复"}\n\nevent: done\ndata: {}\n\n',
        });
      }
      if (
        [
          "/api/v1/catalog",
          "/api/v1/capabilities",
          "/api/v1/governance/reviews",
          "/api/v1/governance/audit",
          "/api/v1/governance/profiles",
        ].includes(p)
      )
        return json({ items: [], has_more: false });
      throw new Error(`Unexpected API: ${request.method()} ${p}`);
    });
    async function choose(label, option) {
      await page.getByRole("combobox", { name: label, exact: true }).click();
      await page.getByRole("option", { name: option, exact: true }).click();
    }
    async function boundedPopup() {
      const box = await page.getByRole("listbox").boundingBox();
      assert(
        box && box.x >= 0 && box.x + box.width <= page.viewportSize().width + 1,
      );
      assert(
        await page
          .getByRole("listbox")
          .evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
      );
    }
    await page.goto(`${app}/hub/settings`);
    await page
      .getByRole("button", { name: "配置模型接口", exact: true })
      .click();
    const protocol = page.getByRole("combobox", {
      name: "接口协议",
      exact: true,
    });
    await protocol.click();
    await page
      .getByRole("option", { name: "Claude（Anthropic）", exact: true })
      .waitFor();
    await boundedPopup();
    await page.screenshot({
      path: path.join(__dirname, "desktop-protocol.png"),
    });
    await page.keyboard.press("Escape");
    assert(await page.getByRole("dialog").isVisible());
    assert(await protocol.evaluate((el) => el === document.activeElement));
    await page
      .getByLabel("API 基础地址", { exact: true })
      .fill("https://relay.example.test/v1");
    await choose("接口协议", "Claude（Anthropic）");
    assert.equal(
      await page.getByLabel("API 基础地址", { exact: true }).inputValue(),
      "https://relay.example.test/v1",
    );
    await choose("接口协议", "OpenAI 兼容");
    await page.getByLabel("API Key", { exact: true }).fill("fixture-only-key");
    await page.getByRole("button", { name: "获取模型", exact: true }).click();
    await page
      .getByRole("checkbox", { name: "relay-chat", exact: true })
      .check();
    await page.getByRole("checkbox", { name: longModel, exact: true }).check();
    await choose("默认模型", longModel);
    await page.getByRole("button", { name: "保存配置", exact: true }).click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal(connection.defaultModel, longModel);
    assert.equal(discoveryPayload.protocol, "openai");
    assert.equal(discoveryPayload.baseUrl, "https://relay.example.test/v1");
    result.checks.push(
      "Dialog popup, Escape focus, protocol switching preserves custom relay URL, discovery and default model saving",
    );
    await page.goto(`${app}/hub/playground`);
    await choose("对话模型", "relay-chat");
    await page.getByLabel("消息", { exact: true }).fill("测试选择的模型");
    await page.getByRole("button", { name: "发送消息", exact: true }).click();
    await page.getByText("模拟模型回复", { exact: true }).waitFor();
    assert.equal(chatPayload.model, "relay-chat");
    result.checks.push(
      "Chat uses selected model and renders simulated streaming reply",
    );
    await page.goto(`${app}/hub/registry/new`);
    await choose("能力类型", "Skill");
    await choose("上线后可见范围", "指定用户");
    await choose("能力类型", "MCP 服务");
    await choose("鉴权方式", "无需鉴权");
    await page.goto(`${app}/hub/registry/create-skill`);
    await choose("创建方式", "专家思维 · 女娲方法");
    await choose("使用模型", "relay-chat");
    await page.goto(`${app}/hub/publications`);
    await choose("筛选状态", "草稿");
    await choose("筛选状态", "全部状态");
    result.checks.push(
      "Registry type/auth/visibility, Skill mode/model, empty-value status reset",
    );
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${app}/hub/settings`);
    await page
      .getByRole("button", { name: "编辑接口配置", exact: true })
      .click();
    assert(
      await page
        .getByRole("dialog")
        .evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
      "Mobile dialog must not overflow horizontally",
    );
    await page.getByRole("combobox", { name: "默认模型", exact: true }).click();
    await page.getByRole("option", { name: longModel, exact: true }).waitFor();
    await boundedPopup();
    await page.screenshot({ path: path.join(__dirname, "mobile-model.png") });
    await page.keyboard.press("Escape");
    assert(await page.getByRole("dialog").isVisible());
    result.checks.push(
      "390px mobile popup fits viewport and long model identifier wraps",
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
