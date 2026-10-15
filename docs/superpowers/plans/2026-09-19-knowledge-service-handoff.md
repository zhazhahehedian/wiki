# 接力：K1 知识服务

更新：2026-10-16。范围见 [知识服务设计](../specs/2026-09-19-knowledge-service-design.md)，任务进度见 [实施计划](2026-09-19-knowledge-service-plan.md)。

## 做到哪

- K1 已实现：`backend/internal/knowledge`、repo 的 knowledge SQL / 向量边界、`internal/http/knowledge_handler.go`、`cmd/server/knowledge.go` 中独立的 River knowledge 队列，以及 `/hub/knowledge` 下的管理、预览、检索、应用页面。未配置 `EMBEDDING_*` 时知识模块不装配，`/api/v1/knowledge/*` 返回 404。
- 检索：初始授权＋重排前、返回前各复核一次；只有关键词命中、未经模型重排确认的片段不返回；审计与响应一致。知识写操作改用行级锁，片段一次批量写入。
- 迁移 0016/0017（0017 读取 `EMBEDDING_DIM`）只在隔离库验证过；业务库仍为 15。
- 2026-10-16 已提交到本地 main（未推送）：市场与 K1 功能、文档精简、review 问题修复与简化、旧 it-wiki 前后端代码删除。旧飞书导入/同步与 RAG 实现可在提交 `c18e34e` 中查阅。
- 验证记录：[K1](../../verification/2026-09-19-knowledge-service/README.md)、[检索优化](../../verification/2026-09-22-knowledge-retrieval/README.md)、[市场](../../verification/2026-09-19-skill-market/README.md)。embedding 均为 fixture。

## 下一步

1. **跑隔离库集成测试**（需要用户授权创建/删除远程测试库）：2026-10-16 改动的批量写入 SQL、行级锁、`KnowledgeBaseReadiness` 尚未在真实 PostgreSQL 上执行过。
2. **待决定**：OpenAI 兼容协议目前总是发送 `max_completion_tokens`、不发送 `max_tokens`，只认 `max_tokens` 的网关（DeepSeek、Ollama、旧版 vLLM 等）会忽略输出上限。
3. **小缺口**：市场详情接口 `/catalog/{slug}` 未返回 `publisher_name`，前端回退显示 open_id。
4. **真实验收**：在根 `.env` 配置平台 `EMBEDDING_BASE_URL/API_KEY/MODEL`（不借用个人游乐场密钥），用 `go run ./cmd/evalknowledge` 跑 baseline 集，再加 `KNOWLEDGE_EVAL_SUITE=optimization KNOWLEDGE_EVAL_REQUIRE_OPTIMIZER=1` 跑优化集；补浏览器文件选择上传与真实账号检查。
5. 推送、业务库迁移到 16/17、部署都需要用户单独授权。

遗留问题：Skill 自定义复杂输入在真实模型下的 503 原因未定位。

## 环境坑

- Windows 路径 `e:/workspace/GoProject/wiki`，WSL 路径 `/mnt/e/workspace/GoProject/wiki`。Go 在 WSL：`/usr/local/go/bin/go`；sqlc：`/home/dpbug/.local/bin/sqlc`；Linux Node：`/home/dpbug/.nvm/versions/node/v24.19.0/bin`，pnpm 9.12.0。
- 仓库 `node_modules` 属于 Windows。WSL 前端验证使用源码副本 `/home/dpbug/.cache/wiki-k1/frontend`（排除 node_modules、.next、.env*），用前确认与源码同步。
- 后端不自动加载根 `.env`；server 启动会跑迁移，`dev-local.sh` 也会启动服务，都不是只读操作。
- `REGISTRY_TEST_DATABASE_URL` 会创建 UUID 隔离库、跑迁移与写入用例后删库；需要对应授权。
- WSL 直连远程 PostgreSQL/MinIO 可用，根配置里的代理在 WSL 测试进程中不可用，验证时只在该进程清除代理变量。远程服务不依赖 Docker。
- WSL Git 与 Windows 换行配置不同，查看 diff 用 `git -c core.autocrlf=true -c core.safecrlf=false diff`，不要批量转换换行符。
