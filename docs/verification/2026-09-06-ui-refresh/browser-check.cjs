// Visual/interaction fixtures only. No real auth, database or model provider used.
// Run with Playwright on NODE_PATH and a frontend using API base http://localhost:13001.
const { chromium } = require("playwright");
const fs = require("node:fs");
const path = require("node:path");
const assert = require("node:assert/strict");
const app = "http://localhost:13001";
const result = {
  fixture: "mock HTTP responses; not runtime acceptance",
  checks: [],
  screenshots: [],
  pageErrors: [],
};
const now = "2026-09-06T08:00:00Z";
const capabilities = [
  {
    id: "demo-mcp",
    slug: "document-search",
    type: "mcp",
    name: "研发文档检索",
    description: "按关键词检索研发文档，返回标题、摘要与原文链接。",
    status: "published",
    department: "研发平台",
  },
  {
    id: "demo-skill",
    slug: "code-review",
    type: "skill",
    name: "代码评审方法",
    description: "整理改动范围、识别边界条件，并生成可执行的评审意见。",
    status: "published",
    department: "基础架构",
  },
  {
    id: "demo-review",
    slug: "release-notes",
    type: "mcp",
    name: "版本发布记录",
    description: "查询项目发布记录，帮助团队定位变更和回滚范围。",
    status: "in_review",
    department: "研发平台",
  },
  {
    id: "demo-draft",
    slug: "weekly-report",
    type: "skill",
    name: "周报整理",
    description: "将本周工作记录整理成进展、问题和下周计划。",
    status: "draft",
    department: "研发平台",
  },
].map((x) => ({
  ...x,
  owner_open_id: "ou_demo",
  visibility: "org",
  revision: 1,
  created_at: now,
  updated_at: now,
  is_live: x.status === "published",
  current_version_id: x.status === "published" ? "v1" : undefined,
  draft_version_id: x.status !== "published" ? "v1" : undefined,
}));
let connection = null;
let chatPayload;
async function routes(route) {
  const request = route.request(),
    url = new URL(request.url()),
    p = url.pathname;
  const json = (data) =>
    route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(data),
    });
  if (p === "/api/v1/auth/me")
    return json({
      id: "demo-user",
      display_name: "林同学",
      email: "demo@example.test",
      avatar_url: "",
      created_at: now,
      updated_at: now,
    });
  if (p === "/api/v1/governance/me")
    return json({
      open_id: "ou_demo",
      is_admin: true,
      department: "研发平台",
      revision: 1,
    });
  if (p === "/api/v1/playground/connection") {
    if (request.method() === "PUT") {
      const { apiKey, ...input } = request.postDataJSON();
      connection = { ...input, hasKey: true, version: input.version + 1 };
    }
    return json(connection);
  }
  if (p === "/api/v1/playground/models")
    return json({ models: ["team-chat", "team-reasoning", "team-vision"] });
  if (p === "/api/v1/playground/chat/stream") {
    chatPayload = request.postDataJSON();
    return route.fulfill({
      status: 200,
      contentType: "text/event-stream",
      body: 'event: delta\ndata: {"text":"建议先明确评审范围，再检查边界条件与异常处理。\\n\\n- 梳理输入和输出\\n- 检查失败路径\\n- 补充必要的验证"}\n\nevent: done\ndata: {}\n\n',
    });
  }
  if (p === "/api/v1/catalog")
    return json({
      items: capabilities.filter((x) => x.is_live),
      has_more: false,
    });
  if (p === "/api/v1/capabilities")
    return json({ items: capabilities, has_more: false });
  if (p === "/api/v1/governance/reviews")
    return json({
      items: capabilities.filter((x) => x.status === "in_review"),
      has_more: false,
    });
  if (p.startsWith("/api/v1/capabilities/")) {
    const item = capabilities.find((x) => x.slug === p.split("/")[4]);
    assert.ok(item, p);
    return json({
      ...item,
      is_owner: true,
      is_admin: true,
      versions: [
        {
          id: "v1",
          version: "1.0.0",
          changelog: "首个版本，支持关键词查询。",
          created_by: "ou_demo",
          created_at: now,
          mcp_endpoint: "https://mcp.example.test/search",
          mcp_transport: "streamable-http",
          mcp_auth_scheme: "bearer",
          tools: [
            {
              name: "search_documents",
              description: "检索研发文档",
              inputSchema: {
                type: "object",
                properties: { query: { type: "string" } },
              },
            },
          ],
        },
      ],
    });
  }
  if (p === "/api/v1/governance/audit" || p === "/api/v1/governance/profiles")
    return json({ items: [], has_more: false });
  throw new Error("Unexpected mock API " + p);
}
(async () => {
  const browser = await chromium.launch({ headless: true });
  try {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 1000 },
    });
    await context.route("**/api/v1/**", routes);
    const page = await context.newPage();
    page.on("pageerror", (e) => result.pageErrors.push(e.message));
    page.setDefaultTimeout(25000);
    async function shot(name) {
      await page.screenshot({
        path: path.join(__dirname, name + ".png"),
        fullPage: true,
      });
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
        name + " overflow",
      );
      result.screenshots.push(name + ".png");
    }
    async function go(route, heading) {
      await page.goto(app + route);
      await page.getByRole("heading", { name: heading, exact: true }).waitFor();
    }
    await go("/hub", "欢迎回来，林同学");
    await shot("dashboard");
    await go("/hub/playground", "游乐场");
    await page.getByRole("button", { name: "接口配置", exact: true }).click();
    await page.getByLabel("API Key", { exact: true }).fill("visual-test-key");
    await page.getByRole("button", { name: "获取模型", exact: true }).click();
    await page
      .getByRole("checkbox", { name: "team-chat", exact: true })
      .check();
    await page
      .getByRole("checkbox", { name: "team-reasoning", exact: true })
      .check();
    await page
      .getByLabel("默认模型", { exact: true })
      .selectOption("team-reasoning");
    await shot("connection-settings");
    await page.getByRole("button", { name: "保存配置", exact: true }).click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    await page.getByRole("button", { name: "模型参数", exact: true }).click();
    await page.getByRole("switch", { name: "启用温度", exact: true }).check();
    await page.getByLabel("温度", { exact: true }).fill("0.4");
    await page
      .getByRole("switch", { name: "启用频率惩罚", exact: true })
      .check();
    await page
      .getByRole("switch", { name: "启用最大 Tokens", exact: true })
      .check();
    await page.getByLabel("最大 Tokens", { exact: true }).fill("1024");
    await page.getByLabel("温度", { exact: true }).scrollIntoViewIfNeeded();
    await shot("playground-parameters");
    await page.keyboard.press("Escape");
    await page
      .getByLabel("消息", { exact: true })
      .fill("如何组织一次代码评审？");
    await page.getByRole("button", { name: "发送消息", exact: true }).click();
    await page.getByText("梳理输入和输出", { exact: true }).waitFor();
    assert.equal(chatPayload.temperature, 0.4);
    assert.equal(chatPayload.frequency_penalty, 0);
    assert.equal(chatPayload.max_tokens, 1024);
    assert.ok(!("top_p" in chatPayload));
    await shot("playground-chat");
    result.checks.push(
      "model discovery, selection and save; enabled parameters; streamed response; Escape closes popover",
    );
    await go("/hub/registry", "注册中心");
    await page.getByRole("link", { name: /研发文档检索/ }).waitFor();
    await shot("registry");
    await page.getByRole("button", { name: "表格视图", exact: true }).click();
    await page.getByRole("table").waitFor();
    result.checks.push("catalog grid and table");
    await go("/hub/publications", "我的发布");
    await page.getByRole("link", { name: /周报整理/ }).waitFor();
    await shot("publications-statuses");
    await go("/hub/reviews", "审核队列");
    await page.getByRole("link", { name: /版本发布记录/ }).waitFor();
    await shot("review-queue");
    await page.getByRole("link", { name: /版本发布记录/ }).click();
    await page
      .getByRole("heading", { name: "版本发布记录", exact: true, level: 1 })
      .waitFor();
    await shot("review-detail");
    await go("/hub/registry/new", "发布能力");
    await shot("create-form");
    await go("/hub/registry/create-skill", "Skill 创建助手");
    await shot("skill-builder");
    await page.setViewportSize({ width: 390, height: 844 });
    await go("/hub/playground", "游乐场");
    await page.getByRole("button", { name: "模型参数", exact: true }).click();
    await page.getByRole("switch", { name: "启用温度", exact: true }).waitFor();
    await shot("mobile-parameters");
    const popup = await page
      .locator('[aria-label="模型参数面板"]')
      .boundingBox();
    assert.ok(
      popup.x >= 0 &&
        popup.x + popup.width <= 390 &&
        popup.y >= 0 &&
        popup.y + popup.height <= 844,
      JSON.stringify(popup),
    );
    await page.keyboard.press("Escape");
    await page.getByRole("button", { name: "接口配置", exact: true }).click();
    await page.getByRole("dialog").waitFor();
    await shot("mobile-connection");
    await page.getByRole("button", { name: "取消", exact: true }).click();
    await page.getByRole("button", { name: "打开导航", exact: true }).click();
    await shot("mobile-navigation");
    await page
      .getByRole("dialog")
      .getByRole("link", { name: "注册中心", exact: true })
      .click();
    await page
      .getByRole("heading", { name: "注册中心", exact: true })
      .waitFor();
    await page.getByRole("link", { name: /研发文档检索/ }).waitFor();
    await shot("mobile-registry");
    result.checks.push(
      "390x844: parameter popover inside viewport; connection dialog; drawer navigation; registry; no horizontal overflow",
    );
    assert.deepEqual(result.pageErrors, []);
  } finally {
    await browser.close();
    fs.writeFileSync(
      path.join(__dirname, "browser-result.json"),
      JSON.stringify(result, null, 2),
    );
  }
  console.log(JSON.stringify(result, null, 2));
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
