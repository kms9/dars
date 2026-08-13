## Why

当前轻量化修改把 `/[workspaceSlug]/agents` 与 `/[workspaceSlug]/squads` 保留成了最小 CRUD 页面，却同时删除了这两个入口原本可达的创建、运行、配置和协作闭环。线上现状、删除前代码与当前实现的对照表明，这已经影响 Agent 的可管理性和 Agent-only Squad 的实际编排能力，因此必须先重新冻结恢复范围和验收契约，再进入实现。

## What Changes

- 恢复 Agent 完整入口流程：可搜索、筛选、排序和批量操作的列表；手工创建、复制与 AI Builder 创建/续接；详情 Overview、Work、Capabilities、Settings；Direct Message、Assign Work、任务历史/转录、取消、归档/恢复。
- Agent Capabilities 只恢复 Instructions 与 Skills：支持编辑智能体指令、绑定/解绑 Workspace Skills、发现并启用/禁用 Runtime local skills；不恢复 MCP 配置和 Integrations。
- 恢复 Agent 的通用设置：头像、运行时与模型参数、权限、并发、加密环境变量、Custom Args，以及适用 Runtime Config。
- 恢复 Squad 完整入口流程：列表检索与展示、创建弹窗、详情资料、Agent-only roster、添加/移除 Agent、成员角色、Leader 原子切换、Agent 状态与活动 Issue、Squad Instructions 和归档。
- 在 Lightweight Web + Server + local Daemon/CLI 边界内补齐上述流程所需的 API、事件、权限、审计和数据模型；不恢复 Desktop，也不整包回滚已删除的完整产品模块。
- 明确保持非目标：Agent MCP 管理、Lark/Slack/其他 Integrations、Human Squad Member、Agent 模板目录、Composio Apps、通用附件系统、Cloud Runtime、Autopilot、完整 Project/Property/Label/Board 和 Inbox/Channel；头像使用受限图片上传能力，Work 页使用 Lightweight Issue 数据面。
- **BREAKING**：更新冻结的 Router manifest、数据库应用表 manifest 与发布门禁；批准基线从 116 条路由/26 张表更新为 126 条路由/27 张表。

## Capabilities

### New Capabilities

- `lightweight-agent-management`: Agent 列表、创建、详情、运行操作、Instructions/Skills 能力配置与 AI Builder 的完整行为契约。
- `lightweight-squad-management`: Agent-only Squad 列表、创建、详情、添加智能体、角色、Leader、Instructions 与归档的完整行为契约。

### Modified Capabilities

- `lightweight-product-boundary`: 将两个入口实际需要的 Agent/Squad 管理能力恢复为目标产品表面，同时继续排除 MCP、Integrations、Human Squad Member 和其他相邻能力。
- `lightweight-collaboration-runtime`: 调整 Agent/Squad 归档与任务生命周期契约，保留 Agent-only Squad 不变量。
- `lightweight-api-security`: 增加 Builder、头像和运行历史所需的授权、审计、幂等与事件要求。
- `lightweight-data-baseline`: 增加 Agent Builder 持久化并更新冻结 schema manifest，不改变 Agent-only `squad_member` 数据模型。
- `lightweight-runtime-release`: 更新 Router/表清单门禁，并把两个入口的真实浏览器端到端流程加入发布矩阵。

## Impact

- Web：`apps/web` 的 Agent/Squad 路由，`packages/views` 页面与共享轻量 Issue/Chat 表面，`packages/core` 查询、状态与协议。
- Server：Agent/Squad/Issue/Task API、Builder 会话、头像上传、运行聚合、权限与审计、Workspace 事件。
- Data：Agent system carrier/Builder draft 持久化与相关并发索引迁移；现有 Agent-only Squad schema 保持；继续禁止 Foreign Key 与 Cascade。
- Runtime：复用现有 local runtime、task queue、transcript 和 Skill discovery，不恢复 Cloud Runtime、MCP 管理或外部集成 connector。
- Release：`make check`、126-route/27-table manifests、权限/租户/密文负向测试，以及本地真实 Daemon 的浏览器验收。
