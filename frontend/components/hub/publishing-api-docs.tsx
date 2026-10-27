import Link from "next/link";
import { PublishingCredentials } from "./publishing-credentials";

const mcp=`# BASE 使用后端地址；同域部署可使用站点地址
export BASE="https://hub.example.com"
# 从安全环境变量或密钥管理服务注入 TOKEN，不写入仓库

curl --fail-with-body "$BASE/api/v1/open/capabilities" \\
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \\
  --data '{"slug":"team-search","type":"mcp","name":"团队搜索",
    "description":"搜索团队内容","visibility":"org","version":"1.0.0",
    "mcp_endpoint":"https://mcp.example.com/mcp",
    "mcp_transport":"streamable-http","mcp_auth_scheme":"bearer","tools":[]}'`;
const skill=`# metadata.json 包含以下 JSON：
{"slug":"report-skill","type":"skill","name":"报告助手",
 "description":"整理报告","visibility":"org","version":"1.0.0"}

# SKILL.md 须有 YAML 文件头：
# ---
# name: report-skill
# description: 整理报告
# ---
# 后接非空指令正文；包内引用的文件须一并上传。

curl --fail-with-body "$BASE/api/v1/open/capabilities" \\
  -H "Authorization: Bearer $TOKEN" \\
  -F 'metadata=<metadata.json;type=application/json' \\
  -F 'files=@SKILL.md;filename=SKILL.md' \\
  -F 'files=@template.md;filename=references/template.md'`;
const submit=`# 从保存响应获取 revision 和 draft_version_id，代入下列字段：
curl --fail-with-body "$BASE/api/v1/open/capabilities/team-search/submit" \\
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \\
  --data '{"revision":1,"version_id":"<draft_version_id>","reason":"提交首版"}'
# 成功返回 status: "in_review"；管理员在站内审核通过后上线。`;
const update=`# 1. 读取最新 revision、status、current_version_id
curl --fail-with-body "$BASE/api/v1/open/capabilities/team-search" -H "Authorization: Bearer $TOKEN"

# 2. 已发布/下线能力先创建草稿（仍需 Owner 身份）
curl --fail-with-body "$BASE/api/v1/open/capabilities/team-search/new-draft" \\
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \\
  --data '{"revision":3,"version_id":"<current_version_id>","reason":"准备新版本"}'

# 3. updated-metadata.json 填完整元数据、响应中的新 revision 和未发布的 version
curl --fail-with-body -X PUT "$BASE/api/v1/open/capabilities/team-search" \\
  -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \\
  --data-binary @updated-metadata.json
# Skill 新版本用 multipart 重新上传完整包；随后用新 revision/draft_version_id 提交。`;

function Code({children}:{children:string}){return <pre className="overflow-x-auto rounded-lg bg-zinc-950 p-4 text-xs leading-6 text-zinc-100"><code>{children}</code></pre>}
export function PublishingAPIDocs(){return <div className="mx-auto max-w-5xl space-y-8 pb-10">
  <header><p className="text-xs font-medium tracking-wider text-blue-600">CAPABILITY HUB API</p><h1 className="mt-2 text-2xl font-semibold">接口文档与 API 凭证</h1><p className="mt-3 text-sm leading-6 text-zinc-500">从脚本或 CI 注册、更新和上传 MCP / Skill，提交至现有审核流程。API 使用绑定账号的权限；账号是 Owner 才能维护其能力，管理员继续在站内审核。</p><nav className="mt-4 flex flex-wrap gap-4 text-sm text-blue-700"><a href="#credentials">管理凭证</a><a href="#endpoints">接口一览</a><a href="#examples">调用示例</a><Link href="/hub/reviews">审核队列</Link></nav></header>
  <PublishingCredentials/>
  <section id="endpoints" className="space-y-4"><h2 className="text-lg font-semibold">鉴权与接口</h2><p className="text-sm leading-6 text-zinc-600">请求头使用 <code>Authorization: Bearer &lt;API 凭证&gt;</code>。以下路径均以 <code>/api/v1/open/capabilities</code> 开头。凭证不能管理其他凭证、调用知识检索或审批上线；没有所需权限返回 403。撤销或过期后，新请求返回 401，已开始处理的请求可能完成。</p>
    <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead><tr className="border-b"><th className="p-3">方法 / 相对路径</th><th className="p-3">用途</th><th className="p-3">权限</th></tr></thead><tbody>{[
      ["GET /","分页列出自己的能力；q、type、status、department、limit（1–100）、offset","read"],
      ["GET /{slug}","详情、revision 和版本 ID；仍校验 Owner/Admin/上线可见范围","read"],
      ["GET /{slug}/versions/{id}/bundle","下载授权可见的 Skill ZIP","read"],
      ["POST /","注册 MCP JSON 或 Skill multipart；返回 201 草稿","write"],
      ["PUT /{slug}","完整更新草稿；必填最新 revision，返回 200","write"],
      ["POST /{slug}/new-draft","已发布或下线后创建新草稿，保留上线版本","write"],
      ["POST /{slug}/submit","提交精确候选版本至审核，返回 200","submit"],
    ].map(([path,purpose,scope])=><tr key={path} className="border-b border-zinc-100"><td className="whitespace-nowrap p-3 font-mono text-xs">{path}</td><td className="min-w-56 p-3 text-zinc-600">{purpose}</td><td className="p-3 font-mono text-xs">capabilities:{scope}</td></tr>)}</tbody></table></div>
  </section>
  <section className="space-y-3"><h2 className="text-lg font-semibold">请求与返回</h2><p className="text-sm leading-7 text-zinc-600">保存必填 <code>slug、type、name、description、visibility、version</code>；创建的 <code>revision</code> 为 0 或省略，更新必须使用读取响应中的 revision。MCP 另需 mcp_endpoint、mcp_transport、mcp_auth_scheme 和 tools；仅支持 streamable-http，鉴权方式 none / bearer。Endpoint 不得含凭证、查询参数或片段。Skill 使用 multipart 的 metadata JSON 和多个 files；相对路径放在 filename 中，最多 50 文件、共 10 MiB。每次上传替换整个草稿包；仅更新同一未发布版本元数据时可不带 files。</p><p className="text-sm leading-7 text-zinc-600">visibility 为 org / department / allowlist。部门使用 <code>department_id</code>（飞书 open_department_id），名称由服务端确认；名单用 <code>allowlist</code> 中的 open_id。所选部门和用户必须在绑定账号的飞书可见范围内，且账号已完成通讯录授权。名称不用于匹配新的部门权限。</p>
    <Code>{`// 保存响应（节选）；读回实际值，勿硬编码 revision 或 ID
{"slug":"team-search","status":"draft","revision":1,
 "draft_version_id":"<uuid>","is_owner":true,"versions":[{"id":"<uuid>","version":"1.0.0"}]}
// 动作请求
{"revision":1,"version_id":"<uuid>","reason":"可选说明"}
// 分页响应
{"items":[],"has_more":false}`}</Code>
  </section>
  <section id="examples" className="space-y-5"><h2 className="text-lg font-semibold">调用示例（Bash / curl）</h2><h3 className="font-medium">注册 MCP</h3><Code>{mcp}</Code><h3 className="font-medium">上传 Skill 完整包</h3><Code>{skill}</Code><h3 className="font-medium">提交审核</h3><Code>{submit}</Code><h3 className="font-medium">更新已发布能力</h3><Code>{update}</Code></section>
  <section className="space-y-3"><h2 className="text-lg font-semibold">错误与重试</h2><p className="text-sm leading-7 text-zinc-600">400：字段、文件路径或 Skill 发布格式不合法；401：凭证缺失、无效、过期或撤销；403：凭证权限、账号角色或飞书目录权限不足；404：资源不存在或对账号不可见；409：revision、状态或版本冲突；502：飞书目录暂不可用。错误为 <code>{'{"error":{"code":"…","message":"…","request_id":"…"}}'}</code>。</p><p className="text-sm leading-7 text-zinc-600">遇到 409 应重新读取并确认差异后提交。网络中断时先 GET 确认保存/提交是否成功，再决定重试；重复创建同一 slug、重复覆盖已发布版本均会冲突。服务不承诺任意写请求可盲目重试。</p></section>
</div>}
