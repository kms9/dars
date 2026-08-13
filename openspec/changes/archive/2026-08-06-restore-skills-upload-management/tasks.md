## 1. Contracts and Server Import/Search

- [x] 1.1 对照参考仓 `skill.go` / `skill_import_archive.go`，在 `server/internal/lightweightapi` 实现 `POST /api/skills/import`（JSON URL + multipart archive）与冲突策略
- [x] 1.2 实现 `GET /api/skills/search`（ClawHub 转发）及上游失败错误语义
- [x] 1.3 移植/补齐路径校验、体积上限、允许主机列表与相关 Go 测试（含 zip-slip / 超限）
- [x] 1.4 在 `lightweight_composition.go`、transport schema 注册上述 2 条路由
- [x] 1.5 更新 `openspec/changes/multica-lightweight-runtime/contracts/routes.txt`（及依赖该清单的测试）使 Router dump = 116

## 2. Core Client and CLI

- [x] 2.1 在 `packages/core/lightweight` 增加 `importSkill`（URL JSON）、archive multipart helper、`searchSkills`
- [x] 2.2 移植 local-skills poll helpers（list/import initiate + poll + timeout），对齐 server pending/running 超时
- [x] 2.3 恢复 CLI `multica skill import`（`--url` / `--file` / on_conflict）与 `multica skill search`
- [x] 2.4 更新相关 builtin skill / CLI 文档中与 import/search 不一致的描述

## 3. Web Skills Create and Import UX

- [x] 3.1 在 `packages/views/lightweight` 实现三种创建入口（Manual / URL / Runtime），参考 `create-skill-dialog.tsx` 但保持轻量组件
- [x] 3.2 实现 URL 粘贴导入与 `.skill`/`.zip` 文件上传（Web P0；ClawHub search UI 可作为同页增强，不阻塞）
- [x] 3.3 实现 Runtime local-skills 发现/多选导入/冲突处理面板，参考 `runtime-local-skill-import-panel.tsx`；非 Owner 显示不可用状态
- [x] 3.4 为 create/import 面板补充 views 级测试（mock lightweight API）

## 4. Web Skills Management UX

- [x] 4.1 用文件树 + 编辑器替换 Skill Detail 的 JSON textarea，保存时调用全量 `replaceSkillFiles`
- [x] 4.2 Skills 列表增加客户端搜索（名称/描述/origin）
- [x] 4.3 Agent 页 Skill 绑定支持 enabled 切换（全量 PUT）；Skill Detail 支持添加到可管理 Agent
- [x] 4.4 删除仍绑定 Skill 时展示冲突并引导先解绑

## 5. Verification

- [x] 5.1 Go：import/search/local-skills 权限与安全路径测试通过
- [x] 5.2 目标 `pnpm typecheck` / 相关 vitest；确认 Web JS 体积门禁未明显失控
- [x] 5.3 增加或扩展 e2e：至少覆盖 URL 或 archive 导入 + Skills 列表可见；有 Daemon 时覆盖 runtime import smoke
- [x] 5.4 Router contract dump = 116，并核对 `POST /api/skills/import` 与 `GET /api/skills/search` 存在
