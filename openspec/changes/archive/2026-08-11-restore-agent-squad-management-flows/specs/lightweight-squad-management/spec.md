## Purpose

定义 Lightweight Web 中 Agent-only Squad 从列表、创建到添加智能体、配置 Instructions、Leader协作和归档的完整行为，且不引入 Human成员或多态 roster。

## ADDED Requirements

### Requirement: Squad 列表支持发现与归档
Squad List SHALL 提供 My/All scope、scope count、名称/描述搜索、Leader/creator/member filter、稳定排序和列展示；表格至少显示 Squad、Leader、Members、Creator，并以头像和 Agent preview表达 roster。My SHALL 只包含当前 actor创建或可管理的可见 Squad，All SHALL 遵守 Workspace权限。

可管理 Squad的行 SHALL 提供 Archive；已归档 Squad不出现在 My/All active scope。本 change不提供 Squad Restore或 Archived scope。

#### Scenario: 搜索 Squad
- **WHEN** Workspace Member在 All scope搜索并按 Leader和 Agent member筛选
- **THEN** 只显示当前 Workspace中可见且满足全部条件的未归档 Squad
- **THEN** count、preview和详情 roster最终一致

### Requirement: Create Squad 原子建立资料、Leader 与 Agent roster
Create Squad SHALL 以 modal或等价聚焦流程收集头像预览、name、description、必填 Leader Agent和可选 additional Agents。Leader与 additional Agents必须属于同 Workspace、未归档且创建者可调用；重复 Agent SHALL 去重或明确拒绝。创建流程不得提供 Human Member选择。

Squad、Leader roster与全部合法 additional Agents SHALL 在一个事务中创建；任一 Agent非法时不得留下部分 Squad。头像上传失败 SHALL 保留已创建 Squad并给出可重试状态。

#### Scenario: 创建多 Agent Squad
- **WHEN** 有权限用户选择合法 Leader和两个 additional Agents创建 Squad
- **THEN** Squad、唯一 Leader与全部 Agent roster在同一事务中可见
- **THEN** Leader也存在于 roster且 role=leader

#### Scenario: 创建包含越权 Agent
- **WHEN** 创建者选择自己不可调用的 private Agent或跨 Workspace Agent
- **THEN** 请求返回403/404，Squad和 roster均不创建

### Requirement: Squad 详情资料与 Instructions 可编辑且防止丢稿
Squad Detail SHALL 展示头像、name、description、creator、created/updated、Leader和 member count，并提供 Members与 Instructions tab。可管理者 SHALL 能编辑资料、头像和 Instructions；其他可见用户只读。Instructions保存后 SHALL 注入 Leader Task context，存在未保存更改时切 tab、离开详情或归档 SHALL 提示保存/放弃/取消。

#### Scenario: 保存 Squad Instructions 后运行
- **WHEN** Squad Manager更新 Instructions并随后创建 Squad Run
- **THEN** Daemon Claim的 Leader context使用已保存的新 Instructions
- **THEN** 未保存的浏览器草稿不会进入 Task context

#### Scenario: Instructions 未保存时离开
- **WHEN** Manager修改 Instructions后尝试返回列表
- **THEN** 页面提供保存、放弃或取消，取消后草稿和当前页面保持不变

### Requirement: Members 支持添加、配置与移除 Agent
Members SHALL 展示 Agent的 name/avatar、role、presence、last active和 active Issue。Squad Manager SHALL 能搜索并添加同 Workspace、未归档且自己可调用的 Agent，编辑非 Leader Agent的自定义 role，移除非 Leader Agent；重复 Agent、跨 Workspace Agent和无调用权限 Agent MUST 被拒绝。

添加、role更新和移除 SHALL 发布 `squad:updated`并在重连 refetch后收敛。页面和 API不得接受或渲染 Human roster member。

#### Scenario: 添加 Agent
- **WHEN** Squad Manager选择一个合法 Workspace Agent并设置 role
- **THEN** Agent加入 roster、显示 role和运行状态，并可由 Leader通过 canonical mention委派

#### Scenario: 非 Manager 修改 roster
- **WHEN** 仅有读取权限的 Member尝试添加、改 role或移除 Agent
- **THEN** Server返回403，roster与 Leader均保持不变

### Requirement: Leader 切换保持单一 Agent Leader 不变量
Squad Leader必须始终是未归档、同 Workspace、可调用的 Agent member。Promote Agent to leader SHALL 在同一事务中锁定 Squad/相关 roster，必要时加入新 Agent，把新成员设为 `leader`、旧 Leader降为普通 Agent role并更新 `squad.leader_id`。当前 Leader不能直接移除；没有合法替代者时不得留下零 Leader。

#### Scenario: Promote 现有 Agent member
- **WHEN** Squad Manager把现有 Agent member Promote为 Leader
- **THEN** 新 Agent成为唯一 Leader，旧 Leader保留为普通 Agent member
- **THEN** 并发读取不会观察到零个或两个 Leader的提交状态

#### Scenario: 移除当前 Leader
- **WHEN** Manager未先切换 Leader就请求移除当前 Leader
- **THEN** Server返回409，Squad与 roster保持不变

### Requirement: Agent member 状态与活动 Issue 来自权威运行数据
Members SHALL 为每个 Agent派生 working、idle、offline、unstable或 archived状态，并显示 last active和当前 active Issue briefs。Agent presence、Task终态与 Issue更新通过 Workspace events驱动 invalidation，断线后 refetch。

#### Scenario: Agent member 开始与结束工作
- **WHEN** roster Agent的 Task从 queued进入 running后完成
- **THEN** Members最终从 working收敛到 idle/offline，并更新 last active
- **THEN** active Issue link只在 Issue/Task仍属于 active状态时展示

### Requirement: Squad Claim 注入 Instructions 与 Agent roster
Squad Leader Task的 Daemon Claim SHALL 返回非 Secret Squad context：id、name、instructions、`members[{agent_id,name,role}]`。Task Core SHALL 验证 Squad未归档、Task Agent属于 roster、roster恰好一个 Leader且 Leader Task Agent与该 Leader一致；roster Agents进入 invocation allowlist和 canonical Agent mention解析。

#### Scenario: Leader 委派 Agent member
- **WHEN** Leader使用 canonical mention指向 roster中的 Agent member
- **THEN** Task Core在同 Workspace/permission范围创建 member Task并保持 Leader-Member-Leader闭环

#### Scenario: 委派非 roster Agent
- **WHEN** Leader尝试 mention不属于当前 Squad的 Agent
- **THEN** Task Core拒绝且不创建 member Task

### Requirement: Squad 可从详情创建关联 Agent
Members中的 Create Agent SHALL 打开 Blank Agent flow并携带只读的 return-to Squad context。Agent成功创建后，若创建者仍可管理 Squad且新 Agent可调用，系统 SHALL 在显式确认后把它加入 roster并返回 Squad详情；取消或创建失败不得改变 roster。

#### Scenario: 从 Squad 创建 Agent
- **WHEN** Squad Manager从 Members选择 Create Agent、完成 Agent创建并确认加入
- **THEN** 新 Agent作为普通 member加入原 Squad，role可继续编辑
- **THEN** 它不会自动替换 Leader

### Requirement: Squad Archive 原子转移当前工作并保留历史
Archive SHALL 仅允许 Squad creator或 Workspace Owner/Admin，并在同一事务中把仍以 Squad为 assignee的当前 Issue转给 Leader Agent、记录 archived_by/time、禁止新 Squad Run。已存在 Task SHALL 保留原 Squad/Leader历史归因并按原 lifecycle收敛；本操作不创建、更新或依赖 Autopilot。

任一 Issue转移或归档写入失败 SHALL 整体回滚。归档后 roster与 Instructions作为历史可供已授权详情/Task渲染，但 Squad不出现在 active list，也不能新增/Promote/Remove member。

#### Scenario: 归档有活动 Issue 的 Squad
- **WHEN** Squad Manager确认归档一个仍有 assigned Issue和 running Leader Task的 Squad
- **THEN** Issue assignee原子转为 Leader，Squad被归档且拒绝新 Run
- **THEN** running Task保留原 Squad历史归因并正常完成或失败

#### Scenario: 转移失败
- **WHEN** 任一待转移 Issue在归档事务中发生并发冲突
- **THEN** Squad、全部 Issue assignee与 roster保持原提交状态，系统返回稳定409
