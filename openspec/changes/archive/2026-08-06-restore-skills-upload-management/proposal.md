## Why

Lightweight 把 Skills 收成最小 CRUD 表单后，后续工作流需要的两类入口都断了：从本机 Runtime 复制 local skills，以及 URL / zip 上传导入。能力并非全部删除——local-skills 后端仍在目标路由内——但 Web/CLI 缺少可用入口，而 `/api/skills/import` 与 `/api/skills/search` 已移出 114 路由白名单。现在需要在不回退完整旧产品的前提下，把上传与管理体验接回 Lightweight Web + Server + CLI。

参考实现：`/Users/logo/self_repo/video_case/fx_coding_case/multica`（尤其 `packages/views/skills/**`、`packages/core/runtimes/local-skills.ts`、`server/internal/handler/skill*.go`、`skill_import_archive.go`）。

## What Changes

- 在 Skills 页恢复三种创建方式：**手动创建**、**Import from URL**、**Copy from runtime**（参考旧 `create-skill-dialog` / `runtime-local-skill-import-panel`）。
- 恢复 Skill 管理可用性：列表搜索、详情文件树编辑（替换 JSON textarea）、Skill↔Agent 绑定与 enable/disable、删除前解绑提示。
- 在 `packages/core/lightweight` 补齐 local-skills initiate/poll helpers，以及 URL/zip import、search 客户端。
- **BREAKING（相对当前 114 路由冻结）**：重新注册并验收
  - `POST /api/skills/import`（JSON URL 导入 + multipart `.skill`/`.zip` 上传）
  - `GET /api/skills/search`（ClawHub 搜索，供 CLI/可选 Web）
  路由总数从 114 调整为 116；同步更新 `contracts/routes.txt`、composition、release 证据。
- 恢复 CLI：`multica skill import`（URL/file）与 `multica skill search`。
- 不恢复：Skill Labels、Agent Template 带 skill、旧 Desktop Views 整包搬迁、全量旧 ListGrid/复杂筛选列体系。

## Capabilities

### New Capabilities
- `lightweight-skills-management`: Lightweight Skills 的创建/导入/文件管理/绑定体验与对应 API·CLI 行为，覆盖 runtime local-skills 与 URL/zip import。

### Modified Capabilities
- `lightweight-api-security`: 路由白名单从 114 扩展为包含 skill import/search；保持其余安全边界。
- `lightweight-product-boundary`: Skill 表面从「最小 List/Create/Detail」扩展为包含导入与可用文件管理，仍不恢复 Labels/Template 等已退出能力。
- `lightweight-runtime-release`: 发布验收的 Router dump 计数与 Skills 相关证据路径随路由与 UI 变更更新。

## Impact

- Server：`lightweightapi` 回补 import/search（可移植参考仓 handler/skill 包逻辑，适配 26 表 baseline）；更新 `lightweight_composition.go`、transport schema、`routes.txt`。
- Web：`packages/views/lightweight/skills.tsx`（及相关组件拆分）、Agent skills 绑定 UI；`packages/core/lightweight` API/helpers。
- CLI：`server/cmd/multica` skill import/search 子命令；内置 skill 文档如涉及则同步。
- Contracts / OpenSpec：与进行中的 `multica-lightweight-runtime` 路由冻结交叉；本 change 显式立项扩展路由。
- 非目标：Desktop、Mobile、Cloud Runtime、Attachments、Labels。
