## 调研范围与证据

调研时间为 2026-08-06（Asia/Shanghai），全程只读。证据来自三层：线上 `/kms9/agents` 与 `/kms9/squads` 的可见交互；当前工作树的 Lightweight Web/Server/Data 实现；基准提交 `736fbc8a5f1b22d48354a0e55baa00661e9e4326` 中删除前的页面、API、服务和测试。未创建 Builder 会话、未保存表单、未归档资源、未绑定外部集成。

线上交互证明“当前可见行为”；历史源码证明删除前的逻辑和边界；二者均不能单独证明当前工作树实现已具备这些能力。当前工作树存在大量未提交改动，本 change 只修改 OpenSpec 文件，没有复原或覆盖用户的业务代码改动。

## 差异与最终范围

| 入口/流程 | 线上与删除前行为 | 当前 Lightweight 行为 | 本 change 最终决策 |
| --- | --- | --- | --- |
| Agent 列表 | My/All/Archived、数量、搜索、多维筛选、排序/列展示、状态/Owner/Access/Runtime/活动/Run count、选择与批量操作、复制/取消/归档/恢复 | 英文卡片式最小列表 | 完整恢复 |
| Agent 创建 | Blank/AI 二选一；手工表单含头像、Instructions、Skills、Runtime/Model/Thinking/Service tier、权限；AI Builder 可续接和自动保存 | 单页最小表单 | 完整恢复；Template catalog 不恢复 |
| Agent 详情与运行 | Overview/Work、DM、Assign Work、任务历史、Issue、transcript、30 天统计 | 单页基础编辑 | 完整恢复，Work 复用 Lightweight Issue 数据面 |
| Agent Capabilities | Instructions、Skills、MCP、Integrations 等 | Env JSON、Skill checklist | 只恢复 Instructions 与 Skills；MCP 和 Integrations 明确不恢复 |
| Agent Settings | General、Access、Environment、Custom Args、适用 Runtime Config | 基础编辑、Env JSON | 恢复与执行管理直接相关的通用设置 |
| Squad 列表/创建 | 搜索/范围/筛选/排序/表格；头像、Leader、Additional members | 内联最小创建表单与卡片 | 完整恢复列表体验；成员限定为 Agent |
| Squad 详情 | Members/Instructions；Agent/Human、role、状态/活动 Issue、Promote/Remove/Add/Create Agent | Agent-only 成员和基础编辑 | 保持 Agent-only；完整恢复添加 Agent、角色、Leader、状态、Instructions 与 Create Agent |
| Lark/Slack/其他集成 | Agent Capabilities 中存在集成入口 | 完全删除 | 不恢复 |

## 依赖闭包

- 可直接复用：Local Runtime、Task Queue、Task Message/Usage、Direct Chat、Lightweight Issue、Skill 管理、Runtime local-skill discovery、Agent Secret encryption、Workspace realtime、现有 Agent-only `squad_member`。
- 必须扩展：Agent 列表/活动聚合、Builder draft/system carrier、头像介质、Issue creator filter、Squad Agent roster 完整操作、Instructions、权限/审计/事件、route/table manifests。
- 不应恢复：MCP 管理 UI/流程、Lark/Slack/其他 Integrations、Human Squad Member、多态 roster、旧 Project/Property/Label/Board、通用 Attachment、Autopilot、Inbox/Channel、Cloud Runtime、Composio、Agent Template、Desktop 适配层。

## 验收边界

恢复完成的判据不是“旧文件重新出现”，而是 Web、Server 与真实本地 Daemon 在新 Lightweight 数据库上完成 spec 中的正向和负向场景。单元测试、静态 diff 或旧源码存在只能作为工程证据，不能替代真实浏览器、HTTP/WS、数据库与 Runtime 执行证据。
