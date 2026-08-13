## MODIFIED Requirements

### Requirement: 保留最小产品表面
Web SHALL 提供邮箱登录、Workspace选择与设置、只读成员列表、Daemon/Runtime、Runtime Profile、Skill、完整 Agent管理、Direct Chat、Agent-only Squad管理、Lightweight Run列表与详情。Run详情 SHALL 将 Flat Comments与 Task Runs分区展示，并以 Issue作为持久化根对象。

Skill表面 SHALL 包含 List/Create/Detail、文件管理、Agent Binding，以及三种创建/导入入口：手动创建、Import from URL（含 archive上传）、Copy from runtime。Agent表面 SHALL 包含 List、Blank/AI Builder Create、Detail Overview/Work/Capabilities/Settings和运行生命周期操作；Capabilities SHALL 只包含 Instructions与 Skills。Squad表面 SHALL 包含 List/Create/Detail、Agent-only roster的添加/移除/角色/Leader管理和 Squad Instructions。该扩展不得恢复 Agent MCP管理、Integrations、Human Squad Member、Skill Labels、Agent Template catalog、Composio、通用 Attachment或其他未列出的外围能力。

#### Scenario: 用户完成主导航闭环
- **WHEN** 已登录用户从目标 Web导航依次访问 Runtime、Agent、Chat、Squad和 Run
- **THEN** 每个保留入口均可访问且只依赖目标 API
- **THEN** Agent/Squad完整流程不依赖 MCP、Integrations、Human Member、Project Board、Issue Table、通用 Attachment、Inbox或混合 Timeline

#### Scenario: 用户完成 Agent 与 Squad 管理
- **WHEN** 已登录用户创建 Agent、配置 Instructions与 Skills，再创建 Agent-only Squad、添加多个 Agent、保存 Squad Instructions、切换 Leader并查看运行历史
- **THEN** 流程在目标 Web、Server、Local Daemon/CLI和 Lightweight database内完整完成
- **THEN** 不出现 MCP、Integrations、Human Member、Template catalog、Composio、Cloud Runtime或 Desktop专属入口

#### Scenario: 用户完成 Skills 导入与管理
- **WHEN** 已登录用户打开 Skills页并使用 Manual、URL/Archive或 Runtime导入创建 Skill，再编辑文件并绑定到 Agent
- **THEN** 流程只依赖目标 Skill/Runtime APIs完成
- **THEN** 不出现 Labels、MCP、Integrations、Template catalog或 Desktop专属入口

### Requirement: 退出完整产品外围能力
目标产品 MUST 移除 Cloud Runtime、通用 Attachment、Human Squad Member、Workspace Invitation、Agent Template catalog、Agent MCP管理、Lark/Slack/其他 Integrations、Composio、Runtime Self-update、Autopilot、Inbox、通用 Channel UI/Router、Billing、Notification、External Webhook、Project/Dashboard/Complex Search、Issue Board/Table、Property、Label、Subscriber、Reaction、Calendar、Child Issue、Quick Action、VCS/PR管理以及相关页面、导航、命令、内置 Skill、API和后台服务。

本 change的受限例外只有 Agent AI Builder与 Agent/Squad专用头像介质。它们 MUST 只暴露 `lightweight-agent-management`与 `lightweight-squad-management`规定的行为，不得构造已退出的 MCP、Integration、Human roster、Template、Attachment、Inbox或 Channel产品面。

#### Scenario: 访问退出能力
- **WHEN** 客户端访问任一仍退出的页面深链、API代表路径或 CLI命令
- **THEN** 系统返回不可用或404，且不会构造旧 Handler或触发旧后台副作用

#### Scenario: 使用受限恢复能力
- **WHEN** 用户使用 Builder或 Agent/Squad avatar
- **THEN** 系统只调用批准的目标 routes/tables/workers完成对应流程
- **THEN** Sidebar、详情 tabs、搜索、API与后台中不出现 MCP、Integrations、Human roster、Template catalog、Attachment browser、Inbox或通用 Channel

#### Scenario: 扫描目标产物
- **WHEN** 发布流程检查 Web route、Sidebar、全局搜索、Modal、API Client、CLI、内置 Skills、i18n、assets和生产依赖
- **THEN** 只存在明确保留或受限恢复能力的入口与依赖，其他退出能力无可执行路径
