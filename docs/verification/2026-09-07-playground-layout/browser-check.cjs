// UI fixtures only; does not contact a real backend or model provider.
// Build/start frontend with NEXT_PUBLIC_API_BASE_URL=http://localhost:13004.
// Run with Playwright on NODE_PATH; APP_URL can override the page URL.
const { chromium } = require("playwright");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const app = process.env.APP_URL || "http://localhost:13004";
const model = "fixture-chat";
const result = {
  fixture:
    "Mock API and controllable SSE ReadableStream; not real model acceptance",
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
    page.on("pageerror", (error) => result.pageErrors.push(error.message));
    let connection = {
      protocol: "openai",
      baseUrl: "https://relay.example.test/v1",
      models: [model, "fixture-reasoning"],
      defaultModel: model,
      hasKey: true,
      version: 1,
    };
    await page.route("**/api/v1/**", async (route) => {
      const p = new URL(route.request().url()).pathname;
      const json = (data) =>
        route.fulfill({
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
          is_admin: false,
          department: "研发平台",
          revision: 1,
        });
      if (p === "/api/v1/playground/connection") return json(connection);
      if (p === "/api/v1/catalog") return json({ items: [], has_more: false });
      throw new Error(`Unexpected API: ${p}`);
    });
    await page.addInitScript(() => {
      const originalFetch = window.fetch;
      window.fetch = async (input, init) => {
        if (!String(input).endsWith("/api/v1/playground/chat/stream"))
          return originalFetch(input, init);
        window.chatPayload = JSON.parse(init.body);
        const encoder = new TextEncoder();
        const stream = new ReadableStream({
          start(controller) {
            window.emitChat = (text) =>
              controller.enqueue(
                encoder.encode(
                  `event: delta\ndata: ${JSON.stringify({ text })}\n\n`,
                ),
              );
            window.finishChat = () => {
              controller.enqueue(encoder.encode("event: done\ndata: {}\n\n"));
              controller.close();
            };
            window.failChat = () =>
              controller.error(new Error("Fixture stream failure"));
            init.signal.addEventListener(
              "abort",
              () => controller.error(new DOMException("Aborted", "AbortError")),
              { once: true },
            );
          },
        });
        return new Response(stream, {
          headers: { "Content-Type": "text/event-stream" },
        });
      };
    });
    const transcript = page.getByLabel("对话消息", { exact: true });
    const input = page.getByLabel("消息", { exact: true });
    const composer = page.getByLabel("消息输入区", { exact: true });
    async function layout(label) {
      const canvasBox = await page
        .getByRole("region", { name: "模型对话", exact: true })
        .boundingBox();
      const inputBox = await composer.boundingBox();
      const viewport = page.viewportSize();
      assert(canvasBox && inputBox);
      assert(
        viewport.height - canvasBox.y - canvasBox.height <= 14,
        `${label}: canvas must fill height`,
      );
      assert(
        inputBox.y + inputBox.height <= viewport.height - 12,
        `${label}: composer visible`,
      );
      assert(
        Math.abs(
          inputBox.x + inputBox.width / 2 - canvasBox.x - canvasBox.width / 2,
        ) < 2,
        `${label}: centered composer`,
      );
      assert(inputBox.width <= 896, `${label}: limited composer width`);
      assert(
        await page.evaluate(
          () =>
            document.documentElement.scrollWidth <= innerWidth &&
            document.documentElement.scrollHeight <= innerHeight + 1,
        ),
        `${label}: document overflow`,
      );
      assert(
        await composer.evaluate((el) => el.scrollWidth <= el.clientWidth + 1),
        `${label}: toolbar overflow`,
      );
      assert(
        await composer.evaluate(
          (el) => getComputedStyle(el).boxShadow !== "none",
        ),
        `${label}: floating shadow`,
      );
      result.checks.push({ label, canvas: canvasBox, composer: inputBox });
    }
    async function atLatest() {
      await page.waitForFunction(() => {
        const el = document.querySelector('[aria-label="对话消息"]');
        return el.scrollHeight - el.scrollTop - el.clientHeight < 2;
      });
    }
    async function send() {
      await input.fill("请说明下面的布局，并给出代码。");
      await page.getByRole("button", { name: "发送消息", exact: true }).click();
      await page
        .getByRole("button", { name: "停止生成", exact: true })
        .waitFor();
    }
    await page.goto(`${app}/hub/playground`);
    await page.getByRole("heading", { name: "开始一场对话" }).waitFor();
    await page
      .getByRole("combobox", { name: "对话模型", exact: true })
      .waitFor();
    assert.equal(
      await page.getByRole("heading", { name: "游乐场", exact: true }).count(),
      0,
    );
    assert.equal(
      await page
        .getByText("选择模型，发送消息，按需调整参数。", { exact: true })
        .count(),
      0,
    );
    await layout("desktop empty 1440x1000");
    await page.screenshot({ path: path.join(__dirname, "desktop-empty.png") });
    await page.setViewportSize({ width: 3840, height: 1929 });
    await layout("wide empty 3840x1929");
    await page.screenshot({ path: path.join(__dirname, "wide-empty.png") });
    await page.setViewportSize({ width: 1440, height: 1000 });
    await send();
    const longReply =
      "# 布局说明\n\n" +
      Array.from(
        { length: 45 },
        (_, i) =>
          `第 ${i + 1} 段：对话画布延伸到页面底部，消息居中排列。输入框悬浮在下方，滚动时可以继续查看之前的内容。\n\n`,
      ).join("") +
      "```html\n" +
      '<div class="chat-layout">示例内容</div>\n'.repeat(30) +
      "```\n\n这是当前回复的最后一段。";
    await page.evaluate((text) => window.emitChat(text), longReply);
    await page.getByText("这是当前回复的最后一段。", { exact: true }).waitFor();
    await atLatest();
    await layout("desktop long response streaming");
    const finalLine = await page
      .getByText("这是当前回复的最后一段。", { exact: true })
      .boundingBox();
    assert(
      finalLine.y + finalLine.height < (await composer.boundingBox()).y - 32,
      "last line clears composer and actions",
    );
    await transcript.evaluate((el) => {
      el.scrollTop = 180;
    });
    await page.getByRole("button", { name: "回到最新消息" }).waitFor();
    const previousTop = await transcript.evaluate((el) => el.scrollTop);
    await page.evaluate(() =>
      window.emitChat("\n\n追加内容不会打断上翻阅读。"),
    );
    await page
      .getByText("追加内容不会打断上翻阅读。", { exact: true })
      .waitFor();
    assert(
      Math.abs(
        (await transcript.evaluate((el) => el.scrollTop)) - previousTop,
      ) < 2,
      "stream does not pull reader back down",
    );
    await page.screenshot({
      path: path.join(__dirname, "desktop-conversation.png"),
    });
    await page.getByRole("button", { name: "回到最新消息" }).click();
    await atLatest();
    await page.evaluate(() => window.finishChat());
    await page.getByRole("button", { name: "发送消息", exact: true }).waitFor();
    result.checks.push(
      "Streaming follows latest; reading position preserved; jump returns to bottom; reply tail unobscured",
    );
    await page.getByRole("button", { name: "模型参数", exact: true }).click();
    const parameters = page.locator('[aria-label="模型参数面板"]');
    await parameters.waitFor();
    const panelBox = await parameters.boundingBox();
    assert(
      panelBox.y >= 0 &&
        panelBox.y + panelBox.height <= page.viewportSize().height,
    );
    await page.getByRole("button", { name: "关闭参数" }).click();
    await page.getByRole("combobox", { name: "对话模型", exact: true }).click();
    await page
      .getByRole("option", { name: "fixture-reasoning", exact: true })
      .click();
    await send();
    assert.equal(
      await page.evaluate(() => window.chatPayload.model),
      "fixture-reasoning",
    );
    await page.evaluate(() => window.emitChat("保留停止前的内容。"));
    await page.getByText("保留停止前的内容。", { exact: true }).waitFor();
    await page.getByRole("button", { name: "停止生成", exact: true }).click();
    await page.getByText("已停止生成", { exact: true }).waitFor();
    assert(
      await page.getByText("保留停止前的内容。", { exact: true }).isVisible(),
    );
    result.checks.push(
      "Parameters and model picker work; stop retains partial response",
    );
    await page.getByRole("button", { name: "新对话", exact: true }).click();
    await page.setViewportSize({ width: 390, height: 844 });
    await layout("mobile empty 390x844");
    await page.screenshot({ path: path.join(__dirname, "mobile-empty.png") });
    await page.getByRole("button", { name: "接口配置", exact: true }).click();
    await page
      .getByRole("dialog", { name: "模型接口配置", exact: true })
      .waitFor();
    await page.getByRole("button", { name: "取消", exact: true }).click();
    await send();
    await page.evaluate((text) => window.emitChat(text), longReply);
    await page.getByText("这是当前回复的最后一段。", { exact: true }).waitFor();
    await atLatest();
    await layout("mobile long response");
    const mobileTail = await page
      .getByText("这是当前回复的最后一段。", { exact: true })
      .boundingBox();
    assert(
      mobileTail.y + mobileTail.height < (await composer.boundingBox()).y - 32,
    );
    await page.evaluate(() => window.failChat());
    await page.getByRole("alert").filter({ hasText: "模型响应失败" }).waitFor();
    await layout("mobile stream failure with retained response");
    await page.screenshot({
      path: path.join(__dirname, "mobile-conversation.png"),
    });
    await page.getByRole("button", { name: "新对话", exact: true }).click();
    for (const viewport of [
      { width: 320, height: 640 },
      { width: 844, height: 390 },
    ]) {
      await page.setViewportSize(viewport);
      await layout(`compact ${viewport.width}x${viewport.height}`);
    }
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole("link", { name: "设置", exact: true }).click();
    await page.getByRole("heading", { name: "设置", exact: true }).waitFor();
    const settingsMain = await page.locator("#hub-content").evaluate((el) => ({
      padding: getComputedStyle(el).padding,
      parentHeight: getComputedStyle(el.parentElement).height,
    }));
    assert.equal(
      settingsMain.padding,
      "28px",
      "normal pages retain original spacing",
    );
    await page.getByRole("link", { name: "游乐场", exact: true }).click();
    await page.getByRole("heading", { name: "开始一场对话" }).waitFor();
    await layout("return navigation");
    connection = null;
    await page.reload();
    await page
      .getByText("先在右上角配置 API 地址、API Key 和模型。", { exact: true })
      .waitFor();
    assert(
      await page
        .getByRole("button", { name: "发送消息", exact: true })
        .isDisabled(),
    );
    await page.getByRole("button", { name: "接口配置", exact: true }).click();
    await page
      .getByRole("dialog", { name: "模型接口配置", exact: true })
      .waitFor();
    result.checks.push(
      "Mobile connection dialog, short viewport, normal-page spacing, route return and unconfigured state",
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
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
