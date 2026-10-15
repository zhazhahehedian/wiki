# 接力：K1 知识服务

更新：2026-10-16。范围见 [知识服务设计](../specs/2026-09-19-knowledge-service-design.md)，任务进度见 [实施计划](2026-09-19-knowledge-service-plan.md)。

## 做到哪

- K1 已实现：`backend/internal/knowledge`、repo 的 knowledge SQL / 向量边界、`internal/http/knowledge_handler.go`、`cmd/server/knowledge.go` 中独立的 River knowledge 队列，以及 `/hub/knowledge` 下的管理、预览、检索、应用页面。旧聊天和 legacy 路由未接回。
- 2026-09-22 按用户要求提前加入问题改写、关键词＋向量混合召回（RRF）和重排，模型不可用时降级并在响应中标明。
- 迁移 0016（权限、应用凭证摘要、版本与片段表）、0017（固定向量维度，读取 `EMBEDDING_DIM`）只在隔离库验证过；业务库仍为 15。
- 内部能力市场、Skill 模板复用、游乐场改动同样未提交。
- 验证记录：[K1](../../verification/2026-09-19-knowledge-service/README.md)、[检索优化](../../verification/2026-09-22-knowledge-retrieval/README.md)、[市场](../../verification/2026-09-19-skill-market/README.md)。embedding 均为 fixture。

## 下一步

1. **修 2026-10-16 review 发现的问题**
   - 默认输出上限 `playground.DefaultOutputTokens = 65536` 超过多数模型上限，改回 4096–8192；`MaxOutputTokens` 降到合理值；OpenAI 兼容网关是否认 `max_completion_tokens` 需确认。
   - `knowledge_repository.go` 的全局 `pg_advisory_xact_lock` 串行了所有知识写操作，去掉，片段改用批量写入。
   - 检索 handler 先写审计再做最终复核，审计可能与响应不符；删掉 handler 里那次复核（service 已做）。
   - 关键词召回结果不经过 `minScore`，可能造成无答案误召回。
   - 前端：应用授权了已取消共享的库后无法保存；文档列表无条件 3 秒轮询；市场详情发布者显示 open_id；可见范围切换后仍提交旧成员 / 部门。
2. **简化**：检索每次请求多轮重复权限复核（每库 4 次查询）、不可能为 nil 的判空、跨层重复校验、游乐场上游错误码分类与多层超时、`knowledge-pages.tsx` 拆分并改用 `useMutation`、删除锁定实现细节的测试。
3. ~~**删除旧 it-wiki 代码**~~（已完成，分支 `cleanup-backend`）：已删 `internal/agent`、`worker`、`eval`、`cmd/evalretrieval`、`docs/eval/`、`service` 中除飞书登录外的部分、http 下 chat/kb/document/chunk/feishu 导入处理器、legacy 路由、llm/parser/splitter/tokenizer/vectorstore、飞书 sheet/bitable loader 及只被它们使用的 SQL；表与迁移保留。`infra/feishu` 的 OAuth、client、docx/wiki loader、URL 解析保留供 K2。旧飞书导入/同步实现见 git 提交 `c18e34e`。
4. **真实验收**：在根 `.env` 配置平台 `EMBEDDING_BASE_URL/API_KEY/MODEL`（不借用个人游乐场密钥），用 `go run ./cmd/evalknowledge` 跑 baseline 集，再加 `KNOWLEDGE_EVAL_SUITE=optimization KNOWLEDGE_EVAL_REQUIRE_OPTIMIZER=1` 跑优化集；补浏览器文件选择上传与真实账号检查。
5. 业务库迁移到 16/17、提交、推送、部署都需要用户单独授权。

遗留问题：Skill 自定义复杂输入在真实模型下的 503 原因未定位。

## 环境坑

- Windows 路径 `e:/workspace/GoProject/wiki`，WSL 路径 `/mnt/e/workspace/GoProject/wiki`。Go 在 WSL：`/usr/local/go/bin/go`；sqlc：`/home/dpbug/.local/bin/sqlc`；Linux Node：`/home/dpbug/.nvm/versions/node/v24.19.0/bin`，pnpm 9.12.0。
- 仓库 `node_modules` 属于 Windows。WSL 前端验证使用源码副本 `/home/dpbug/.cache/wiki-k1/frontend`（排除 node_modules、.next、.env*），用前确认与源码同步。
- 后端不自动加载根 `.env`；server 启动会跑迁移，`dev-local.sh` 也会启动服务，都不是只读操作。
- `REGISTRY_TEST_DATABASE_URL` 会创建 UUID 隔离库、跑迁移与写入用例后删库；需要对应授权。
- WSL 直连远程 PostgreSQL/MinIO 可用，根配置里的代理在 WSL 测试进程中不可用，验证时只在该进程清除代理变量。远程服务不依赖 Docker。
- WSL Git 与 Windows 换行配置不同，查看 diff 用 `git -c core.autocrlf=true -c core.safecrlf=false diff`，不要批量转换换行符。
