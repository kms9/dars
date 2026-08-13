# lightweight-agent-management Specification

## Purpose
定义 Lightweight Web 中从 Agent 列表、创建入口到详情运行与配置的完整可观察行为，使用户可管理本地 Runtime Agent，并明确把能力配置收敛为 Instructions 与 Skills。
## Requirements
### Requirement: Agent 列表支持发现、范围与批量管理
Agent List SHALL 展示 My、All、Archived scope及各自数量，并支持名称/描述搜索、availability、access、runtime、owner筛选、稳定排序、列显示和选择。默认表格 SHALL 至少显示 Agent、Status、Owner、Access、Runtime、Last active、Run count；筛选与排序不得使无权查看的 Agent泄露到计数或结果。

行操作 SHALL 按状态和权限提供 Duplicate、Cancel work、Archive或 Restore。批量 Access、Archive、Restore SHALL 对每个目标执行同一单资源授权，并返回可识别的成功/失败明细；部分失败不得被报告为全部成功。

#### Scenario: 搜索与筛选 Agent
- **WHEN** Workspace Member在 All scope搜索并组合 availability、access、runtime、owner filter
- **THEN** 表格、scope count和分页只包含该成员可见且满足条件的 user Agent
- **THEN** system carrier不出现在任何 scope、count、filter option或搜索结果

#### Scenario: 批量操作包含无权目标
- **WHEN** Member批量归档自己拥有的 Agent和一个无管理权限的 Agent
- **THEN** 系统只归档获准目标并逐项返回 forbidden失败
- **THEN** UI不显示“全部成功”且 refetch后与服务端一致

### Requirement: 手工创建与复制提供完整可编辑草稿
Create Agent入口 SHALL 先让用户选择 Start from blank或 Create with AI。Blank flow SHALL 支持头像预览、name、description、Instructions、Workspace Skills、Local Runtime、model、thinking、service tier、max concurrency，以及 private/workspace/specified members access；name和同 Workspace可绑定 Runtime为创建必填。

手工草稿 SHALL 在同一浏览器会话的路由往返中恢复，并在成功创建后清理。Create and open SHALL 创建 Agent后导航到详情；头像上传失败 SHALL 保留已创建 Agent、显示可重试状态且不得谎报头像成功。

Duplicate SHALL 预填原 Agent的用户可编辑非 Secret配置，但 MUST NOT 复制 Agent identity、owner、archive/status、运行历史、Env Secret或现有 session；原 Runtime不再可绑定时 SHALL 要求重新选择。

#### Scenario: 手工创建完整 Agent
- **WHEN** 有权限用户填写合法 Blank draft、配置 Instructions与 Skills、选择在线可绑定 Local Runtime并执行 Create and open
- **THEN** 系统创建一个 `kind=user` Agent，绑定所选 Skills/access/config，并打开该 Agent详情
- **THEN** 普通列表和可调用目标中只出现最终 Agent，不出现任何创建辅助资源

#### Scenario: 复制 Agent
- **WHEN** 用户对可见 Agent选择 Duplicate
- **THEN** 创建页预填 Instructions、Skills和其他非 Secret行为配置并要求新的 name
- **THEN** Env Secret、历史、owner与 identity均为空或重新生成

### Requirement: AI Builder 会话可自动保存、切换 Runtime 与续接
Create with AI SHALL 只接受同 Workspace、在线且用户可绑定的 Local Runtime，以及该 Runtime报告兼容的 model。创建 Builder session后，系统 SHALL 提供私有对话、结构化 Agent draft、未发送编辑自动保存、Runtime switch和 unfinished session resume。Builder draft可生成 Instructions和 Skills绑定建议，但不得增加 MCP或 Integration配置。

Builder对话 SHALL 使用可靠 Task lifecycle；Runtime switch在存在 active builder task时 MUST 返回 409。完成创建 SHALL 原子验证最新 draft、创建普通 Agent、标记 Builder session完成并使其不再出现在 unfinished list；失败 SHALL 保留可续接 draft。Builder carrier、chat和 transcript MUST NOT 出现在普通 Agent、Direct Chat、run count、activity或 invocation target中。

#### Scenario: 续接未完成 Builder
- **WHEN** 用户离开一个已有对话和自动保存 draft的 Builder页面后重新打开 Create with AI
- **THEN** unfinished list展示该用户在当前 Workspace的 session
- **THEN** 打开后恢复对话、draft、Runtime和未发送编辑，不暴露其他用户或 Workspace的 session

#### Scenario: Builder 完成创建
- **WHEN** Builder输出通过协议校验且只包含批准 Agent字段的 draft，用户确认创建
- **THEN** 一个普通 Agent及其 Instructions/Skills与其他获准配置在同一完成操作中创建
- **THEN** session从 unfinished list消失，重复完成不会创建第二个 Agent

#### Scenario: Active Builder task 时切换 Runtime
- **WHEN** 当前 Builder turn尚处于 queued、dispatched、waiting_local_directory或 running
- **THEN** Runtime switch返回稳定 409且原 Runtime、task与 draft保持不变

### Requirement: Agent 详情提供 Overview 与直接运行操作
Agent Detail header SHALL 展示头像、name、presence/workload、description、model、runtime、access、updated time，并按权限提供 Direct Message、Assign Work、Cancel work、Archive/Restore。Overview SHALL 展示 current work、稳定分页的 recent work、Issue link、transcript、失败原因、initiator，以及 owner/access/runtime/model/concurrency/skills摘要和过去30天 run count、success rate、average duration、fail count。

Direct Message SHALL 打开或创建该 Agent的 Direct Chat；Assign Work SHALL 以该 Agent为 assignee创建 Lightweight Issue/Run。历史统计只根据持久化 Task/Usage计算，不得把 queued当 success或把 system Builder task计入。

#### Scenario: 从 Overview 查看失败运行
- **WHEN** 用户打开一个含失败 Task的 Agent Overview并选择该条 recent work
- **THEN** 页面显示持久化 failure、时间、initiator和关联 Issue/Chat
- **THEN** View transcript打开按序的 Task Messages，而不是构造模拟结果

#### Scenario: Direct Message 与 Assign Work
- **WHEN** 有调用权限的 Member分别点击 Direct Message和 Assign Work
- **THEN** 前者进入该 Agent的 Direct Chat，后者创建并打开以该 Agent为 assignee的 Lightweight Run

### Requirement: Work 页按 Assigned 与 Created 发现 Lightweight Issue
Work SHALL 支持搜索、Assigned/Created scope、状态筛选、稳定排序和分页。Assigned只匹配 `assignee_type=agent, assignee_id=<agent>`；Created只匹配可信 `creator_type=agent, creator_id=<agent>`，不得通过客户端伪造 creator。结果 SHALL 使用 Lightweight Issue字段并可打开 Issue Detail，不依赖 Project、Property、Label、Board/Table或复杂搜索服务。

#### Scenario: 查看 Agent 创建的工作
- **WHEN** 用户切换到 Created scope并选择状态 filter
- **THEN** 只返回该 Agent作为可信 creator且状态匹配的可见 Issue
- **THEN** cursor翻页期间使用稳定顺序且不混入 Assigned-only Issue

### Requirement: Capabilities 只提供 Instructions 与 Skills
Agent Capabilities SHALL 只展示 Instructions和 Skills两个子页，不展示 MCP或 Integrations入口、深链、操作或空占位。Instructions编辑 SHALL 明确保存；存在未保存更改时切换 tab、离开详情或切换 Agent SHALL 提示保存/放弃/取消。

Skills SHALL 区分 Workspace Skills与绑定 Runtime发现的 local skills，并支持查看详情、绑定/解绑 Workspace Skill、启用/禁用 runtime skill；无管理权限时只读。Runtime离线、不支持 local skill discovery或 discovery失败 SHALL 显示不同状态，不得把失败误报为“没有 Skill”。保存结果 SHALL 与后续 Task解析到的 skill bundle一致。

#### Scenario: 带未保存 Instructions 切换 tab
- **WHEN** Manager修改 Instructions后未保存并切换到 Skills或 Settings
- **THEN** 页面提供保存、放弃或取消选择
- **THEN** 取消保持原 tab和草稿，放弃恢复服务端值，保存成功后才切换

#### Scenario: 配置 Workspace 与 Runtime Skills
- **WHEN** Agent Manager绑定 Workspace Skill并禁用一个 Runtime local skill
- **THEN** UI显示权威绑定与 disabled状态
- **THEN** 后续 Task的 resolved skill bundle包含 Workspace Skill且不包含被禁用的 runtime skill

#### Scenario: 访问未恢复的能力深链
- **WHEN** 用户尝试打开该 Agent的 MCP或 Integrations view参数或旧深链
- **THEN** 页面回退到批准 view或显示不可用，且不调用相关旧 API

### Requirement: Settings 完整管理资料、访问与执行配置
Settings SHALL 提供 General、Access、Environment、Custom Args，并仅对支持的 Runtime提供 Runtime Config。General SHALL 管理头像、name、description、Runtime、model、thinking、service tier、max concurrency；Access SHALL 管理 private/workspace/specified members；Environment SHALL 只向 Agent owner或 Workspace Owner/Admin Reveal/Update并写审计；Custom Args与 Runtime Config SHALL 校验 Runtime schema和类型。

切换 Runtime/model SHALL 重新校验绑定权限和兼容性；配置保存不得覆盖并发更新，版本冲突 SHALL 返回409并要求 refetch。无权限用户可以查看允许的非 Secret摘要，但编辑控件 SHALL 只读或隐藏。

#### Scenario: Reveal 与更新 Environment
- **WHEN** Agent owner Reveal Env、更新一个 key并删除另一个 key
- **THEN** Reveal/Update各产生带 actor、agent、time、action的 audit记录
- **THEN** 列表、普通详情和 Realtime只显示 key/redacted metadata

#### Scenario: 保存不兼容 model
- **WHEN** 用户把 Agent model改为目标 Runtime未报告的值
- **THEN** Server返回稳定 invalid argument/conflict，原配置保持不变

### Requirement: Agent 生命周期操作保持任务与历史一致
Cancel work SHALL 幂等取消该 Agent所有 active tasks并返回取消数量；Agent Archive SHALL 在一个原子 operation中执行相同取消、记录 archived_by/time并禁止新调用。Restore SHALL 恢复可见和可调用资格但不得重启已取消任务。Agent有 active tasks不得使 Archive退化为通用 `active_tasks_exist`拒绝。

归档/恢复、status、Instructions、Skills和 Settings更新 SHALL 发布 non-Secret Workspace events；客户端断线重连 SHALL refetch snapshot、detail、tasks和 skills。所有操作 MUST 验证 Workspace、资源管理权限和幂等/并发状态。

#### Scenario: 归档正在工作的 Agent
- **WHEN** Agent Manager确认归档一个存在 running与 queued Task的 Agent
- **THEN** active tasks全部收敛为 cancelled，Agent被归档且新调用被拒绝
- **THEN** 历史 Task、messages、usage、Issue/Chat归因仍可查看

#### Scenario: 恢复 Agent
- **WHEN** Agent Manager恢复一个 Runtime仍可绑定的 archived Agent
- **THEN** Agent重新出现在 active scope并可接收新工作
- **THEN** 归档时取消的任务保持终态且不会自动重放

