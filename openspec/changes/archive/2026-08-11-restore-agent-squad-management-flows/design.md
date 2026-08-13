## Context

见 [proposal.md](./proposal.md) 的动机和 [research.md](./research.md) 的差异证据。当前目标分支已把 Agent/Squad 页面改为 `packages/views/lightweight` 的最小 CRUD，并删除了原 `packages/views/agents`、`packages/views/squads` 和相当一部分共享依赖；Server 仍保留 Local Runtime、Task Queue、Chat、Issue、Skill、Agent Secret 和 Agent-only Squad 基础 API。

设计必须遵守现有目标边界：只交付 Web + Server + local Daemon/CLI；React Query 管 Server state，Zustand 只管 view/client state；`packages/core` 不使用 react-dom/localStorage/process.env，`packages/ui` 不依赖 core，`packages/views` 不依赖 Next；数据库无 Foreign Key/Cascade，所有索引用单独的 concurrent migration；`make check` 使用隔离数据库。

## Goals / Non-Goals

**Goals:**

- 以用户从两个入口能完成的行为为恢复单位，补齐 UI、API、数据、权限、事件与真实运行闭环。
- Agent 能配置 Instructions 与 Skills；Squad 能添加 Agent、配置 Instructions 并完成 Leader-Member-Leader 协作。
- 尽量复用 Lightweight 的 Issue/Chat/Task/Skill/Runtime 核心，保持 route/table manifest 可审计。

**Non-Goals:**

- 不对当前大范围未提交改动执行 git 回滚，也不把基准提交的目录整体复制回来。
- 不恢复 Agent MCP 管理、Lark/Slack/其他 Integrations、Human Squad Member或多态 roster。
- 不兼容 Desktop，不恢复 Template catalog、Composio、Autopilot、通用 Attachment、Project/Property/Label/Board 或 Inbox/Channel UI。
- 不把旧线上数据或页面截图当作新实现已经正确运行的证据。

## Decisions

### 1. 选择行为级移植，不做目录级回滚

旧 Agent/Squad 组件可作为交互和测试参考，但实现只移植通过本 change specs 的依赖闭包。列表、详情和创建页面在 `packages/views` 重建；Server state 查询/协议放在 `packages/core`；Next 路由只做 platform wiring。

旧组件直接依赖已退出的 MCP/Integration、Issue Surface、Attachment、Project/Label/Property、Channel 等模块，整体恢复会重新引入明确排除的产品面，因此不采用目录级回滚。

### 2. Agent 列表使用一致性快照，详情历史使用稳定分页

新增 workspace Agent snapshot，单次返回 scope counts、presence/workload、run count、last activity 和过滤所需的非 Secret 元数据；搜索、排序、scope、filter 与 hidden columns 保持 URL/View store 状态。详情任务历史按 `updated_at DESC, id DESC` 使用 opaque cursor，30 天指标从同一权威 Task/Usage 数据聚合。Work 页扩展现有 Issue list 的 `creator_type/creator_id` 查询，不恢复完整 Issue Surface。

相较于列表逐行查询多个端点，该方案避免 N+1 和跨请求状态撕裂；相较于新建 rollup worker，它不增加后台常驻任务。

### 3. AI Builder 复用 Chat/Task，使用隐藏 system carrier 和一张 draft 表

`agent` 增加 `kind=user|system` 与唯一 `system_key`；每个 Builder session 使用 `kind=system, system_key=agent_builder:<flow-id>` 的 carrier、现有 `chat_session/chat_message/agent_task_queue` 执行对话，并在 `agent_builder_draft` 持久化最后确认和未发送草稿。所有普通 Agent/Chat 查询必须排除 system carrier。

Builder 创建、runtime switch、draft save 使用独立 session API。Runtime switch 只在没有 active builder task 时进行；完成创建在事务中从验证后的 draft 创建普通 Agent、标记 session 完成并清理 draft。请求失败保留可续接草稿。

### 4. Agent Capabilities 只包含 Instructions 与 Skills

Capabilities 恢复两个子页：Instructions 负责编辑行为指令并提供离开 dirty guard；Skills 负责 Workspace Skill 绑定/解绑和 Runtime local skills 的发现、详情、启用/禁用。Task Claim 继续从权威绑定和 disabled runtime skills 解析最终 skill bundle。

不恢复 MCP tab、MCP editor、Runtime MCP discovery 展示或 Integrations tab。现有底层字段若仍为其他目标契约所需，本 change 不新增其用户入口、路由或依赖，也不将其计入 Agent 管理验收。

### 5. 通用 Settings 复用现有目标配置与 Secret 边界

Settings 保留 General、Access、Environment、Custom Args 和适用 Runtime Config。Env 继续使用单独 Reveal/Update、Agent owner/Workspace Owner/Admin 权限和 activity audit；普通列表、详情摘要、日志与 Realtime不返回 Secret。Runtime/model 切换重新校验绑定权限与兼容性。

### 6. 头像是受限介质，不恢复通用 Attachment

新增 Agent/Squad 专用 multipart avatar action，限制 MIME、字节数、像素和解码结果；Server 生成不可猜测、不可变的对象 key，更新资源 `avatar_url`，替换时同步清理旧对象。公开 media GET只返回已绑定头像并设置 immutable cache；Workspace 删除显式枚举清理对象。

创建页先用本地预览创建资源，再调用专用 avatar action；上传失败保留已创建资源并显示可重试状态。该协议不会创建 Attachment 表、评论附件或文件浏览入口。

### 7. Squad 沿用 Agent-only roster，补齐 Agent 与 Instructions 操作

保持现有 `squad_member(agent_id, role)`；Leader 仍由 `squad.leader_id` 指向 Agent，且 roster 中恰有同一 Agent、`role=leader` 的记录。添加成员只接受同 Workspace、未归档且操作者可调用的 Agent；切换 Leader 在同一事务和锁范围内自动加入新 Leader、降级旧 Leader；当前 Leader不可直接删除。

Squad Detail 提供 Members 与 Instructions。Instructions 保存后注入 Leader Task Claim；Daemon Claim roster 只包含 Agent，因此现有 canonical Agent mention 和 Leader-Member-Leader 协议可直接复用，不增加 Human 分支或 schema migration。

### 8. 归档语义按入口原流程恢复并原子化

Agent Archive 在同一 service operation 内取消 active tasks、写入终态/事件并归档；显式 Cancel Work可单独执行。Restore不恢复已取消任务。Squad Archive原子把当前仍指向 Squad 的 Issue assignee转给 Leader、保留 Task/Squad历史归因并归档 Squad；不触碰已退出的 Autopilot。归档后的 Squad不可接收新 Run，本 change不增加 Squad Restore。

### 9. Route/schema manifest 是源码真值，批准基线为 126 路由与 27 表

在原 116 路由上新增 10 条：Agent snapshot/history/cancel 3 条、Builder 4 条、Agent/Squad avatar与 media 3 条。数据库在原 26 表上只新增 `agent_builder_draft`；其他需求通过现有表加列完成，现有 `squad_member` 保持不变，因此批准总数为 27。

实现必须维护排序后的 checked-in route/schema manifests；contract test比较集合和总数。任何实施中发现必须新增 route/table的情况都先更新 OpenSpec，而不是绕过门禁。

### 10. 权限、事件与批量操作共享单资源规则

Server 对 Agent owner、Squad creator、Workspace Owner/Admin的规则保持单一判定函数；批量操作逐项使用同一规则并返回成功/失败明细。Realtime只发 non-Secret invalidation payload，客户端按 `event_id` 幂等并在重连后 refetch snapshot/detail。

## Risks / Trade-offs

- [旧组件依赖闭包再次膨胀] → 以 package-boundary test、目标 import allowlist和 bundle comparison阻止 MCP/Integration/Project/Attachment/Channel回流。
- [Builder system carrier泄露到普通列表或 Chat] → 数据查询默认 `kind=user`，增加跨 list/search/summary的 contract tests和 Workspace删除清理测试。
- [Instructions 或 Skills 页面状态与执行不一致] → 保存后 refetch权威配置，并用真实 Task Claim/skill bundle验证。
- [Squad Leader 与 roster并发失配] → 锁 Squad/目标成员并在单事务复验，不依赖 FK或最终一致性事件。
- [Archive取消/转移发生部分写入] → Task terminalization、Issue reassignment、resource archive与事件 publication共享事务边界；失败整体回滚。
- [126/27数字在实现中漂移] → checked-in manifest是真值，OpenSpec变更是修改清单的唯一入口。

## Migration Plan

1. 在隔离 Lightweight数据库应用 `agent.kind/system_key`、`agent_builder_draft`和 avatar相关可回滚 migrations；每个索引使用独立 `CREATE INDEX CONCURRENTLY` migration。
2. 回填现有 `agent.kind=user`并验证所有普通查询排除 system carrier；`squad_member`不做结构迁移。
3. 先发布兼容读取和新 API，Web仍可使用旧最小页面；验证 126-route/27-table manifests、租户与 Secret contract。
4. 按 Agent list/detail/manual、Builder、Instructions/Skills、Squad的顺序启用 Web，逐阶段运行真实浏览器和 Daemon测试。
5. 切换失败时先回退 Web；Server保留新增字段读取，停止 Builder新写后再回滚可逆 migration。已有 Builder draft在回滚前导出或显式清理，不做隐式数据丢弃。
