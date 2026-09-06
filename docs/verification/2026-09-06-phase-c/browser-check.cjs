// Run against the opt-in disposable registry fixture, never a production server.
// NODE_PATH must resolve Playwright; no project dependency is added.
const { chromium } = require("playwright");
const fs = require("node:fs");
const path = require("node:path");
const assert = require("node:assert/strict");
const app = "http://localhost:13000";
const api = "http://localhost:18089";
const output = __dirname;
const result = { checks: [], pageErrors: [], screenshots: [] };
let browser;
async function shot(page, name) {
  await page.screenshot({ path: path.join(output, name), fullPage: true });
  result.screenshots.push(name);
}
async function checkText(page, text) {
  await page.getByText(text, { exact: true }).first().waitFor();
}
async function action(page, label, reason = "") {
  await page.getByRole("button", { name: label, exact: true }).click();
  if (reason)
    await page.getByRole("textbox", { name: /操作说明/ }).fill(reason);
  const response = page.waitForResponse(
    (r) =>
      r.url().startsWith(api + "/api/v1/capabilities/") &&
      r.request().method() === "POST",
  );
  await page.getByRole("button", { name: "确认" + label, exact: true }).click();
  const r = await response;
  assert.equal(r.status(), 200, await r.text());
  // Mutation includes query invalidation before its dialog closes.
  await page
    .getByRole("button", { name: "确认" + label, exact: true })
    .waitFor({ state: "hidden" });
}
async function go(page, url) {
  await page.goto(app + url);
  await page.getByRole("main").waitFor();
}
async function save(page) {
  const response = page.waitForResponse(
    (r) =>
      r.url().startsWith(api + "/api/v1/capabilities") &&
      ["POST", "PUT"].includes(r.request().method()),
  );
  await page.getByRole("button", { name: "保存草稿", exact: true }).click();
  const r = await response;
  assert.ok([200, 201].includes(r.status()), await r.text());
  await page.waitForURL(/\/hub\/registry\/[^/]+$/);
  await page.getByRole("button", { name: "提交审核", exact: true }).waitFor();
}
(async () => {
  browser = await chromium.launch({ headless: true });
  const people = {};
  for (const role of ["owner", "admin", "consumer", "outsider"]) {
    const context = await browser.newContext({
      viewport: { width: 1440, height: 1000 },
    });
    const page = await context.newPage();
    page.setDefaultTimeout(30000);
    page.on("pageerror", (e) =>
      result.pageErrors.push(role + ": " + e.message),
    );
    await page.goto(api + "/__test/session?as=" + role);
    await page
      .getByRole("heading", { name: "注册中心", exact: true })
      .waitFor();
    people[role] = { context, page };
  }
  const owner = people.owner.page,
    admin = people.admin.page,
    consumer = people.consumer.page,
    outsider = people.outsider.page;
  const slug = "phase-c-browser-search";
  const detail = "/hub/registry/" + slug;
  await go(owner, "/hub/registry/new");
  await owner.getByLabel("名称", { exact: true }).fill("研发文档检索");
  await owner.getByLabel("唯一标识", { exact: true }).fill(slug);
  await owner.getByLabel("部门", { exact: true }).fill("研发平台");
  await owner
    .getByLabel("能力描述", { exact: true })
    .fill("按关键词查询研发文档，返回标题与文档链接。");
  await owner
    .getByLabel("Endpoint", { exact: true })
    .fill("https://mcp.example.test/search/v1");
  await owner
    .getByLabel("工具清单 JSON", { exact: true })
    .fill(
      JSON.stringify(
        [
          {
            name: "search_documents",
            description: "检索研发文档",
            inputSchema: {
              type: "object",
              properties: { query: { type: "string" } },
            },
          },
        ],
        null,
        2,
      ),
    );
  await save(owner);
  await action(owner, "提交审核");
  await checkText(owner, "审核中");
  await go(admin, "/hub/reviews");
  await admin.getByRole("link", { name: /研发文档检索/ }).waitFor();
  await shot(admin, "review-queue.png");
  await admin.getByRole("link", { name: /研发文档检索/ }).click();
  await admin.getByText("search_documents", { exact: true }).click();
  await shot(admin, "review-detail.png");
  await action(admin, "驳回", "请在说明中补充查询结果格式。");
  await go(owner, detail);
  await owner.getByText(/请在说明中补充查询结果格式/).waitFor();
  await shot(owner, "owner-rejected.png");
  await owner.getByRole("link", { name: "编辑草稿" }).click();
  await owner
    .getByLabel("能力描述", { exact: true })
    .fill("按关键词查询研发文档，返回 JSON 数组，包含标题、摘要与文档链接。");
  await save(owner);
  await action(owner, "提交审核");
  await go(admin, detail);
  await action(admin, "通过并上线");
  await go(consumer, "/hub/registry");
  await consumer.getByRole("link", { name: /研发文档检索/ }).click();
  await checkText(consumer, "已上线");
  assert.equal(
    await consumer
      .getByRole("button", { name: "通过并上线", exact: true })
      .count(),
    0,
  );
  assert.equal(
    await consumer.getByRole("link", { name: "编辑草稿" }).count(),
    0,
  );
  await shot(consumer, "consumer-published.png");
  result.checks.push(
    "Owner creates MCP, admin rejects, owner revises, admin approves, consumer sees published content",
  );
  await go(owner, detail);
  await action(owner, "发布新版本");
  await owner.waitForURL(/\/edit$/);
  await owner.getByLabel("名称", { exact: true }).fill("研发文档检索 v2");
  await owner.getByLabel("版本号", { exact: true }).fill("2.0.0");
  await owner
    .getByLabel("Endpoint", { exact: true })
    .fill("https://mcp.example.test/search/v2");
  await owner
    .getByLabel("版本说明", { exact: true })
    .fill("新增部门限定访问，保留原版直至审核通过。");
  await owner
    .getByLabel("上线后可见范围", { exact: true })
    .selectOption("department");
  await save(owner);
  await action(owner, "提交审核");
  await go(outsider, detail);
  await outsider
    .getByRole("heading", { name: "研发文档检索", exact: true })
    .waitFor();
  await checkText(outsider, "https://mcp.example.test/search/v1");
  await go(admin, detail);
  await admin.getByRole("heading", { name: "本次提交与上线版本" }).waitFor();
  await shot(admin, "version-comparison.png");
  await action(admin, "通过并上线");
  await go(outsider, detail);
  await outsider
    .getByRole("alert")
    .filter({ hasText: "能力或版本不存在" })
    .waitFor();
  result.checks.push(
    "Old snapshot remains visible during review; approved department scope blocks users without trusted department",
  );
  const identityResponse = await people.consumer.context.request.get(
    api + "/api/v1/governance/me",
  );
  const identity = await identityResponse.json();
  await go(admin, "/hub/profiles");
  await admin.getByRole("textbox", { name: "搜索用户" }).fill(identity.open_id);
  await admin.getByRole("button", { name: "搜索", exact: true }).click();
  const form = admin
    .locator("form")
    .filter({
      has: admin.getByRole("button", { name: "保存部门", exact: true }),
    });
  await form.getByLabel("可信部门", { exact: true }).fill("研发平台");
  await form
    .getByLabel("核实与变更说明", { exact: true })
    .fill("验收账号已核实属于研发平台。");
  const savedProfile = admin.waitForResponse(
    (r) => r.url().endsWith("/department") && r.request().method() === "PUT",
  );
  await form.getByRole("button", { name: "保存部门", exact: true }).click();
  assert.equal((await savedProfile).status(), 200);
  await form.getByLabel("核实与变更说明", { exact: true }).waitFor();
  await shot(admin, "trusted-department.png");
  await go(consumer, detail);
  await consumer
    .getByRole("heading", { name: "研发文档检索 v2", exact: true })
    .waitFor();
  await checkText(consumer, "https://mcp.example.test/search/v2");
  await consumer.setViewportSize({ width: 390, height: 844 });
  await shot(consumer, "mobile-consumer.png");
  assert.ok(
    await consumer.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
    "mobile horizontal overflow",
  );
  result.checks.push(
    "Admin assigns trusted department with audit and consumer gains matching access; mobile view has no overflow",
  );
  await go(owner, detail);
  await action(owner, "下线能力", "检索服务计划维护。");
  await go(consumer, detail);
  await consumer
    .getByRole("alert")
    .filter({ hasText: "能力或版本不存在" })
    .waitFor();
  await go(owner, detail);
  await action(owner, "提交审核", "维护结束申请恢复。");
  await go(admin, detail);
  await action(admin, "通过并上线");
  await go(admin, "/hub/audit");
  await admin
    .getByRole("combobox", { name: "审计动作" })
    .selectOption("approve");
  await admin.getByRole("button", { name: "筛选日志" }).click();
  await admin.locator("details").first().waitFor();
  await admin.locator("details").first().locator("summary").click();
  await shot(admin, "audit-filtered.png");
  await go(consumer, "/hub/audit");
  await consumer.getByRole("heading", { name: "需要管理员权限" }).waitFor();
  result.checks.push(
    "Owner offline blocks consumers; resubmission restores after approval; audit filters and role gate work",
  );
  // The cloud fixture published this Skill through the same service and MinIO.
  await go(admin, "/hub/registry/governed-skill");
  await admin.getByText("SKILL.md", { exact: false }).first().waitFor();
  await admin
    .getByText("references/template.md", { exact: false })
    .first()
    .waitFor();
  await shot(admin, "skill-file-review.png");
  await go(consumer, "/hub/registry/governed-skill");
  const downloadPromise = consumer.waitForEvent("download");
  await consumer.getByRole("link", { name: "下载 Skill 包" }).click();
  const download = await downloadPromise;
  assert.equal(await download.failure(), null);
  result.checks.push(
    "Published Skill file contents render and authorized consumer downloads MinIO bundle",
  );
  assert.deepEqual(result.pageErrors, []);
  fs.writeFileSync(
    path.join(output, "browser-result.json"),
    JSON.stringify(result, null, 2) + "\n",
  );
  console.log(JSON.stringify(result, null, 2));
})()
  .catch(async (e) => {
    result.error = e.stack;
    fs.writeFileSync(
      path.join(output, "browser-result.json"),
      JSON.stringify(result, null, 2) + "\n",
    );
    console.error(e);
    process.exitCode = 1;
  })
  .finally(async () => {
    if (browser) await browser.close();
  });
