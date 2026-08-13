## MODIFIED Requirements

### Requirement: Squad 归档保护历史与活动任务
`DELETE /api/squads/{squadId}` SHALL 表示归档而非物理删除。归档 SHALL 在同一事务把仍以 Squad为 assignee的 Issue转给 Leader Agent并标记 Squad archived；归档后 MUST 拒绝新分配。已经存在的 Task MUST 保留原 Squad/Leader历史归因并继续按可靠 lifecycle收敛，不得因 assignee转移被重启或重复执行。P0不提供 Restore，也不得创建或更新已退出的 Autopilot。

#### Scenario: 归档活动 Squad
- **WHEN** Agent-only Squad仍有 assigned Issue或 active Task，且有权限用户确认归档
- **THEN** Issue assignee与 Squad archive在一个事务中提交，Issue改为 Leader Agent
- **THEN** active Task保留原 Squad attribution并正常完成、失败或取消

#### Scenario: 归档空闲 Squad
- **WHEN** Agent-only Squad仅有终态或无 Task，且有权限用户确认归档
- **THEN** 当前 Issue assignee与 Squad archive在一个事务中提交，Issue改为 Leader Agent
- **THEN** 历史 Task保留原 Squad attribution，Squad不可接收新 Run

#### Scenario: 归档事务冲突
- **WHEN** Issue转移或 Squad状态在事务内发生冲突
- **THEN** 返回稳定409，Squad、Issue assignee与 Task均保持原提交状态

#### Scenario: 归档后创建 Run
- **WHEN** 客户端尝试把新 Issue分配给已归档 Squad
- **THEN** 系统拒绝且不创建 Task
