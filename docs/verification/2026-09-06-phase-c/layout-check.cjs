// Run after browser-check.cjs against the same disposable fixture.
const { chromium } = require("playwright");
const fs = require("node:fs");
const out = __dirname;
(async () => {
  const b = await chromium.launch({ headless: true });
  try {
    const c = await b.newContext({ viewport: { width: 1440, height: 1000 } }),
      page = await c.newPage();
    page.setDefaultTimeout(30000);
    await page.goto("http://localhost:18089/__test/session?as=owner");
    const api = "http://localhost:18089/api/v1",
      slug = "phase-c-browser-search";
    let d = await (await c.request.get(api + "/capabilities/" + slug)).json();
    const headers = {
      Origin: "http://localhost:13000",
      "X-CSRF-Token": "csrf",
    };
    let r = await c.request.post(api + "/capabilities/" + slug + "/new-draft", {
      headers,
      data: {
        revision: d.revision,
        version_id: d.current_version_id,
        reason: "",
      },
    });
    if (r.status() !== 200) throw new Error(await r.text());
    d = await r.json();
    const v = d.versions.find((v) => v.id === d.current_version_id);
    const input = {
      slug,
      type: d.type,
      name: "研发文档检索 v3",
      description: d.description,
      department: d.department,
      visibility: d.visibility,
      allowlist: d.allowlist ?? [],
      revision: d.revision,
      version: "3.0.0",
      changelog: "扩展检索能力，等待审核确认。",
      mcp_endpoint: v.mcp_endpoint,
      mcp_transport: v.mcp_transport,
      mcp_auth_scheme: v.mcp_auth_scheme,
      tools: v.tools,
    };
    r = await c.request.put(api + "/capabilities/" + slug, {
      headers,
      multipart: { metadata: JSON.stringify(input) },
    });
    if (r.status() !== 200) throw new Error(await r.text());
    d = await r.json();
    r = await c.request.post(api + "/capabilities/" + slug + "/submit", {
      headers,
      data: {
        revision: d.revision,
        version_id: d.draft_version_id,
        reason: "",
      },
    });
    if (r.status() !== 200) throw new Error(await r.text());
    await page.goto("http://localhost:18089/__test/session?as=admin");
    await page.goto("http://localhost:13000/hub/reviews");
    await page.getByRole("link", { name: /研发文档检索 v3/ }).waitFor();
    const button = page.getByRole("button", { name: "搜索", exact: true });
    if (
      (await button.evaluate((e) => getComputedStyle(e).whiteSpace)) !==
      "nowrap"
    )
      throw new Error("search button still wraps");
    await page.screenshot({ path: out + "/review-queue.png", fullPage: true });
    const consumer = await b.newContext();
    await consumer.request.get(
      "http://localhost:18089/__test/session?as=consumer",
      { maxRedirects: 0 },
    );
    const who = await (
      await consumer.request.get(api + "/governance/me")
    ).json();
    await page.goto("http://localhost:13000/hub/profiles");
    await page.getByRole("textbox", { name: "搜索用户" }).fill(who.open_id);
    await page.getByRole("button", { name: "搜索", exact: true }).click();
    await page.getByText(who.open_id, { exact: true }).waitFor();
    await page.getByLabel("可信部门", { exact: true }).waitFor();
    await page.screenshot({
      path: out + "/trusted-department.png",
      fullPage: true,
    });
    await page.setViewportSize({ width: 390, height: 844 });
    if (
      !(await page.evaluate(
        () => document.documentElement.scrollWidth <= innerWidth,
      ))
    )
      throw new Error("profile mobile overflow");
    await page.screenshot({
      path: out + "/mobile-department.png",
      fullPage: true,
    });
    const result = JSON.parse(
      fs.readFileSync(out + "/browser-result.json", "utf8"),
    );
    result.checks.push(
      "Final production build: governance search buttons remain horizontal; department admin page also fits 390px",
    );
    result.screenshots.push("mobile-department.png");
    fs.writeFileSync(
      out + "/browser-result.json",
      JSON.stringify(result, null, 2) + "\n",
    );
    console.log("final search layout and mobile department checked");
  } finally {
    await b.close();
  }
})().catch((e) => {
  console.error(e);
  process.exitCode = 1;
});
