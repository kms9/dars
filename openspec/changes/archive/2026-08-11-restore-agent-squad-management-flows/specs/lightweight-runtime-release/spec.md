## REMOVED Requirements

### Requirement: 路由、Worker 和数据表计数是硬门禁
**Reason**: 原 requirement把 Router和 schema固定为116/26，与本 change批准的10条恢复路由和1张 Builder draft表冲突。

**Migration**: 以新增的“Route、Worker和 schema manifests是硬门禁”替代，批准总数更新为126/27；现有 Worker allowlist保持不变，不增加 Integration worker。

## ADDED Requirements

### Requirement: Route、Worker 和 schema manifests 是硬门禁
发布 SHALL 要求 Router dump与 checked-in 126条 route manifest精确相等、运行 Worker与既有批准清单精确相等、`pg_catalog`与 checked-in 27表 schema manifest精确相等。页面出现、单元测试通过、自描述 health或只检查总数不得替代集合级证据。

#### Scenario: 任一清单漂移
- **WHEN** Router多/少/替换任一路由、运行中出现额外 Worker，或数据库应用表集合不是批准27表
- **THEN** 发布门禁失败并输出集合 diff

#### Scenario: 恢复路由计入门禁
- **WHEN** 发布流程 dump Router
- **THEN** 结果包含 Agent snapshot/history/cancel、Builder和 avatar的全部10条 delta
- **THEN** 总数精确为126，且 MCP、Integrations、Human Squad Member、Agent Template、Composio、Attachment、Inbox/Channel routes不存在

### Requirement: Agent 与 Squad 入口流程进入真实发布验收
发布验收 SHALL 使用真实 PostgreSQL、独立 Server、独立 Local Daemon/Runtime和目标 Web浏览器覆盖两个入口的正向与负向矩阵。至少覆盖：Agent list scope/search/filter/sort/batch；Blank/Duplicate/AI Builder create/resume/runtime switch/finalize；Overview/Work/transcript/DM/Assign/Cancel/Archive/Restore；Instructions/Skills/Env/Args；Squad list/create/detail、添加/移除 Agent、role、Leader switch、Instructions、Create Agent和 archive transfer。

验收必须同时检查 Web、HTTP/WS、数据库、Daemon/Runtime handler的同一生命周期；mock、静态 DOM或旧源码不能单独标记流程通过。MCP、Integrations和 Human Squad Member必须作为负向范围验证不可达。

#### Scenario: 核心 Agent/Squad 浏览器闭环
- **WHEN** 浏览器用户创建 Agent、配置 Instructions与 Skills、产生 Chat/Issue work、创建多 Agent Squad、保存 Squad Instructions、切换 Leader并归档
- **THEN** UI、API、Task lifecycle、database、transcript、events和 Runtime执行结果一致
- **THEN** Archive后历史仍可追溯，system carrier/Secret/跨 Workspace对象无泄露

#### Scenario: Builder 恢复与幂等完成
- **WHEN** 用户中断并续接 Builder，会话完成请求被重放
- **THEN** draft/transcript正确恢复且只创建一个普通 Agent
- **THEN** carrier不出现在 Agent/Chat/activity/run count中

#### Scenario: Instructions 与 Skills 进入真实执行
- **WHEN** 用户更新 Agent Instructions、绑定 Workspace Skill、禁用 Runtime Skill后创建真实 Run
- **THEN** Daemon Claim和 resolved skill bundle使用已保存 Instructions/Skills状态
- **THEN** MCP或 Integration配置入口不参与该流程

## MODIFIED Requirements

### Requirement: 删除能力以负向证据验收
发布验收 SHALL 覆盖仍退出能力的页面、导航、深链、API、CLI、内置 Skill、依赖和后台 side effect；代表路径 MUST 返回404，旧模块不得被间接构造或通过动态入口恢复。Desktop专属页面和构建不在本阶段验收范围。

Builder与 Agent/Squad avatar SHALL 从退出矩阵移入正向/安全矩阵；Agent MCP管理、Lark/Slack/其他 Integrations、Human Squad Member、Agent Template、Composio、通用 Attachment、Inbox/Channel、Cloud Runtime、Autopilot等仍属于负向矩阵。

#### Scenario: 负向矩阵
- **WHEN** 自动化遍历冻结退出族的代表页面、路径、命令和事件
- **THEN** 所有仍退出入口均不可用，且日志、Router、Worker和依赖扫描无旧能力执行证据

#### Scenario: 防止相邻模块随恢复能力回流
- **WHEN** 验收使用 Builder、avatar、Agent Instructions/Skills或 Squad Agent roster/Instructions
- **THEN** 流程成功，但 MCP/Integrations/Human roster/Template/Attachment browser/Inbox/Channel/Composio代表入口仍为404且相关 Worker未启动
