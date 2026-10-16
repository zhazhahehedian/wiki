# 接力：K1 知识服务

更新：2026-10-16。范围见 [知识服务设计](../specs/2026-09-19-knowledge-service-design.md)，任务进度见 [实施计划](2026-09-19-knowledge-service-plan.md)。

## 做到哪

- K1 已实现：`backend/internal/knowledge`、repo 的 knowledge SQL / 向量边界、`internal/http/knowledge_handler.go`、`cmd/server/knowledge.go` 中独立的 River knowledge 队列，以及 `/hub/knowledge` 下的管理、预览、检索、应用页面。未配置 `EMBEDDING_*` 时知识模块不装配，`/api/v1/knowledge/*` 返回 404。
- 检索：初始授权＋重排前、返回前各复核一次；只有关键词命中、未经模型重排确认的片段不返回；审计与响应一致。知识写操作改用行级锁，片段一次批量写入。
- 迁移 0016/0017（0017 读取 `EMBEDDING_DIM`）只在隔离库验证过；业务库仍为 15。
- 2026-10-16 已推送到 origin/main：市场与 K1 功能、文档精简、review 问题修复与简化、旧 it-wiki 前后端代码删除，见[推送说明](../../releases/2026-10-16-market-knowledge-cleanup.md)。旧飞书导入/同步与 RAG 实现可在提交 `c18e34e` 中查阅。
- 验证记录：[K1](../../verification/2026-09-19-knowledge-service/README.md)、[检索优化](../../verification/2026-09-22-knowledge-retrieval/README.md)、[市场](../../verification/2026-09-19-skill-market/README.md)。embedding 均为 fixture。

## 下一步：验证配置 embedding 后的知识服务

前提：server 启动会对 `DATABASE_URL` 自动跑迁移，业务库仍为 15。下面用一个**新建的独立验证库**，不碰业务库。

1. **准备配置**（根 `.env`，不提交）
   - `EMBEDDING_BASE_URL`（含 `/v1` 等前缀）、`EMBEDDING_API_KEY`、`EMBEDDING_MODEL`、`EMBEDDING_DIM`。用平台自己的 key，不借用个人游乐场 key。
   - `EMBEDDING_DIM` 必须等于模型实际输出维度。0017 会把它固定进 `knowledge_chunks.embedding vector(N)`，server 启动时发现不一致会拒绝启动；之后改维度需要新库或重建迁移。
   - 可选：`KNOWLEDGE_OPTIMIZER_BASE_URL/API_KEY/MODEL` 三项同时设置，启用问题改写和模型重排。不配时只返回达到向量阈值的结果，纯关键词命中不返回。
   - `KNOWLEDGE_MIN_SCORE` 先用默认 0.5，评测后再调。
2. **先跑隔离库集成测试**：在 `backend/` 执行 `KNOWLEDGE_TEST_DISPOSABLE=1 go test ./internal/http/ ./internal/repo/ -run Knowledge -v`。它会在远程 PostgreSQL 上新建并最后删除一个 UUID 库；加 `KNOWLEDGE_TEST_S3=1` 会同时写入并清理 MinIO 测试对象。测试读取根 `.env`，embedding 用 fixture，用来确认 2026-10-16 改动的批量写入、行级锁和就绪查询在真实 PostgreSQL 上可用。
3. **起一套指向验证库的服务**：在同一 PostgreSQL 上新建一个库（如 `capability_hub_k1_eval`），只在启动进程里把 `DATABASE_URL` 指向它，然后启动后端和前端（见 README）。启动会把该库迁移到 17。
4. **页面上走一遍**（`/hub/knowledge`）：建库，上传 `backend/internal/knowledge/testdata/` 下的 `expense.md`、`leave.txt`、`travel.md`，确认索引完成、解析预览和诊断正常，再在"检索测试"页试几个问题。另建一个应用，授权这个库并生成凭证。
5. **跑评测**：在 `backend/` 设置 `KNOWLEDGE_API_BASE`（如 `http://localhost:8080`）、`KNOWLEDGE_APPLICATION_TOKEN`、`KNOWLEDGE_EVAL_BASE_IDS`（库 ID），执行 `go run ./cmd/evalknowledge`。配置了优化模型的话，再加 `KNOWLEDGE_EVAL_SUITE=optimization KNOWLEDGE_EVAL_REQUIRE_OPTIMIZER=1` 跑一次。结果、模型名和阈值记录到 `docs/verification/` 下的新目录。
6. 根据评测结果校准 `KNOWLEDGE_MIN_SCORE`。验证通过后，业务库迁移到 16/17 需另行确认。

## 其他待办

- **待决定**：OpenAI 兼容协议目前总是发送 `max_completion_tokens`、不发送 `max_tokens`，只认 `max_tokens` 的网关（DeepSeek、Ollama、旧版 vLLM 等）会忽略输出上限。
- **小缺口**：市场详情接口 `/catalog/{slug}` 未返回 `publisher_name`，前端回退显示 open_id。
- Skill 自定义复杂输入在真实模型下的 503 原因未定位。
- 部署需另行确认。

## 环境坑

- 通用：后端不自动加载根 `.env`；server 启动和 `dev-local.sh` 都会跑迁移，不是只读操作。远程 PostgreSQL/MinIO 不依赖本地 Docker。根 `.env` 与 `frontend/.env.local` 不在 Git 里，换电脑需要自己带过去或重新填写。
- 原家用 Windows 机器（换电脑后按实际环境核对）：
  - Windows 路径 `e:/workspace/GoProject/wiki`，WSL 路径 `/mnt/e/workspace/GoProject/wiki`。Go 在 WSL：`/usr/local/go/bin/go`；sqlc：`/home/dpbug/.local/bin/sqlc`；Linux Node：`/home/dpbug/.nvm/versions/node/v24.19.0/bin`，pnpm 9.12.0。
  - 仓库 `node_modules` 属于 Windows。WSL 前端验证使用源码副本 `/home/dpbug/.cache/wiki-k1/frontend`（排除 node_modules、.next、.env*），用前确认与源码同步。
  - WSL 直连远程 PostgreSQL/MinIO 可用；根配置里的代理在 WSL 测试进程中不可用，验证时只在该进程清除代理变量。
  - WSL Git 与 Windows 换行配置不同，查看 diff 用 `git -c core.autocrlf=true -c core.safecrlf=false diff`，不要批量转换换行符。
