// UI verification with intercepted business APIs, not live provider acceptance.
const { chromium } = require('playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const app = process.env.APP_URL || 'http://localhost:13019';
const result = { fixture: 'mock business APIs; real production frontend', checks: [], pageErrors: [] };
const source = {
  id: 'skill-1', slug: 'weekly-report', type: 'skill', name: '团队技术周报',
  description: '适用于研发团队每周汇报。汇总项目进展、风险与下周计划，输出结构清晰的 Markdown 周报。',
  department: '研发', owner_open_id: 'ou_author', publisher_name: '林同学', status: 'published', is_live: true,
  visibility: 'org', current_version_id: 'live-1', published_version: '1.2.0', revision: 0,
  created_at: '2026-09-18T08:00:00Z', updated_at: '2026-09-19T07:00:00Z',
  versions: [{ id: 'live-1', version: '1.2.0', has_bundle: true, changelog: '补充风险清单与周报示例', published_at: '2026-09-19T07:00:00Z' }],
};
const instructions = '# 技术周报\n\n## 输入要求\n提供本周完成事项、项目风险与下周安排。\n\n## 使用步骤\n1. 归纳进展与结果。\n2. 列出风险、负责人及下一步。\n3. 按[周报模板](references/template.md)输出。\n\n## 输出示例\n| 项目 | 进展 | 风险 |\n| --- | --- | --- |\n| 能力市场 | 完成模板复用 | 待团队验收 |';
const draft = { slug: 'weekly-report-copy-ab123456', name: source.name, description: source.description, instructions, files: [{ path: 'references/template.md', content: '# 周报模板\n\n## 本周进展\n## 风险与协作\n## 下周计划' }] };
const files = [{ name: 'SKILL.md', size: Buffer.byteLength(instructions), content: `---\nname: weekly-report\ndescription: report\n---\n${instructions}` }, { name: draft.files[0].path, size: 80, content: draft.files[0].content }];

(async () => {
  const browser = await chromium.launch({ headless: true, ...(process.env.BROWSER_PATH ? { executablePath: process.env.BROWSER_PATH } : {}) });
  try {
    for (const [device, viewport] of [['desktop', { width: 1440, height: 1000 }], ['mobile', { width: 390, height: 844 }]]) {
      const page = await browser.newPage({ viewport });
      page.setDefaultTimeout(10000);
      page.on('pageerror', e => result.pageErrors.push(e.message));
      const requests = [];
      let saved;
      let denied = false;
      let unsupported = false;
      await page.route('**/api/v1/**', async route => {
        const request = route.request(), url = new URL(request.url()), p = url.pathname;
        requests.push({ method: request.method(), path: p, query: url.search });
        const headers = { 'access-control-allow-origin': app, 'access-control-allow-credentials': 'true', 'access-control-allow-headers': 'content-type,x-csrf-token', 'access-control-allow-methods': 'GET,POST,OPTIONS' };
        if (request.method() === 'OPTIONS') return route.fulfill({ status: 204, headers });
        let status = 200, data;
        if (p === '/api/v1/auth/me') data = { id: 'reader', display_name: '陈同学' };
        else if (p === '/api/v1/governance/me') data = { open_id: 'ou_reader', is_admin: false, department: '研发', revision: 1 };
        else if (p === '/api/v1/playground/connection') data = null;
        else if (p === '/api/v1/catalog') {
          const items = [source, ...['需求审查', '代码检查', '数据分析', '会议纪要', '发布说明'].map((name, i) => ({ ...source, id: `item-${i}`, slug: `item-${i}`, name, description: `将团队的${name}方法整理为可复用的工作流程，附输入要求与交付示例。` }))];
          const q = url.searchParams.get('q');
          data = { items: q ? items.filter(x => `${x.name} ${x.description}`.includes(q)) : items, has_more: !q && Number(url.searchParams.get('offset') ?? 0) === 0 };
        } else if (p === '/api/v1/catalog/weekly-report') {
          data = source;
          if (denied) { status = 404; data = { error: { code: 'capability_not_found', message: '能力或版本不存在' } }; }
        } else if (p === '/api/v1/capabilities/weekly-report/versions/live-1/files') data = { files };
        else if (p === '/api/v1/capabilities/weekly-report/versions/live-1/bundle') return route.fulfill({ headers: { ...headers, 'content-disposition': 'attachment; filename="weekly-report.zip"' }, contentType: 'application/zip', body: Buffer.from('mock download') });
        else if (p === '/api/v1/catalog/weekly-report/template') {
          assert.equal(url.searchParams.get('version_id'), 'live-1');
          data = { source: { slug: source.slug, name: source.name, version: '1.2.0', version_id: 'live-1' }, draft };
          if (unsupported) { status = 422; data = { error: { code: 'skill_template_unsupported', message: '此 Skill 无法完整复制，请下载原包使用' } }; }
        } else if (p === '/api/v1/skill-builder/drafts') { saved = JSON.parse(request.postData()).draft; status = 201; data = { ...source, slug: saved.slug, name: saved.name, status: 'draft', is_live: false, is_owner: true, current_version_id: '', draft_version_id: 'copy', versions: [{ id: 'copy', version: '1.0.0', has_bundle: false }] }; }
        else if (p === `/api/v1/capabilities/${draft.slug}`) data = { ...source, slug: draft.slug, name: saved?.name ?? source.name, status: 'draft', is_live: false, is_owner: true, current_version_id: '', draft_version_id: 'copy', versions: [{ id: 'copy', version: '1.0.0', has_bundle: false }] };
        else throw new Error(`Unexpected API ${request.method()} ${p}`);
        return route.fulfill({ status, headers, contentType: 'application/json', body: JSON.stringify(data) });
      });
      const noOverflow = async () => assert(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth + 1), `${device} horizontal overflow`);
      await page.goto(`${app}/hub/registry`);
      await page.getByRole('heading', { name: '团队技术周报' }).waitFor();
      await noOverflow();
      await page.screenshot({ path: path.join(__dirname, `${device}-market.png`), fullPage: true });
      await page.getByRole('button', { name: '下一页' }).click();
      await page.getByRole('button', { name: '周报', exact: true }).click();
      await page.waitForFunction(() => document.querySelectorAll('h2').length === 1);
      assert(requests.some(x => x.query.includes('q=') && x.query.includes('type=skill') && x.query.includes('offset=0')));
      await page.getByRole('link').filter({ has: page.getByRole('heading', { name: '团队技术周报' }) }).click();
      await page.getByRole('heading', { name: '输入要求' }).waitFor();
      await noOverflow();
      await page.screenshot({ path: path.join(__dirname, `${device}-detail.png`), fullPage: true });
      await page.getByRole('button', { name: /references\/template.md/ }).click();
      await page.getByRole('heading', { name: '周报模板' }).waitFor();
      const downloadPromise = page.waitForEvent('download');
      await page.getByRole('link', { name: '下载 Skill 包' }).click();
      assert.equal((await downloadPromise).suggestedFilename(), 'weekly-report.zip');
      await page.getByRole('link', { name: '基于此创建' }).click();
      await page.getByLabel('Skill 名称').waitFor();
      assert.equal(await page.getByLabel('附件 1 内容').inputValue(), draft.files[0].content);
      await page.getByLabel('Skill 名称').fill('我的团队周报');
      await page.getByLabel('附件 1 内容').fill('自定义周报模板');
      await noOverflow();
      await page.screenshot({ path: path.join(__dirname, `${device}-template.png`), fullPage: true });
      await page.getByRole('button', { name: '保存为我的草稿' }).click();
      await page.waitForURL(`**/hub/registry/${draft.slug}`);
      assert.equal(saved.slug, draft.slug);
      assert.equal(saved.files[0].content, '自定义周报模板');
      assert(!requests.some(x => x.path === '/api/v1/skill-builder/turn'));
      assert.equal(requests.filter(x => x.path === '/api/v1/catalog/weekly-report/template').length, 1, 'save must not refetch and replace the template');
      unsupported = true;
      await page.goto(`${app}/hub/registry/create-skill?source=weekly-report&version=live-1`);
      await page.getByRole('alert').waitFor();
      assert.equal(await page.getByLabel('Skill 名称').count(), 0);
      denied = true;
      await page.goto(`${app}/hub/registry/weekly-report/market`);
      await page.getByRole('alert').waitFor();
      assert.equal(await page.getByRole('link', { name: '基于此创建' }).count(), 0);
      result.checks.push(`${device}: filters, published detail, attachments, download, template edit/save without model, unsupported template, denied source, no overflow`);
      await page.close();
    }
    assert.deepEqual(result.pageErrors, []);
    fs.writeFileSync(path.join(__dirname, 'result.json'), JSON.stringify(result, null, 2));
    console.log(JSON.stringify(result, null, 2));
  } finally { await browser.close(); }
})().catch(e => { console.error(e); process.exitCode = 1; });
