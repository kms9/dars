# lightweight-product-boundary Specification

## Purpose
定义 DARS 轻量版作为单一产品的交付边界、保留体验、退出能力与构建范围，防止实现过程中恢复完整产品功能或引入兼容模式。
## Requirements
### Requirement: 单一轻量产品替换当前复制版本
仓库 SHALL 只交付一个 Lightweight 产品形态，不得提供 Full/Light 运行模式、功能开关或同进程兼容层。目标产品 SHALL 以 Web、Server 和 Local Daemon/CLI 构成可运行闭环。

#### Scenario: 启动目标产品
- **WHEN** 操作者按目标发行配置启动系统
- **THEN** 仅启动 Lightweight Web、Server 和 Local Daemon/CLI 所需组件
- **THEN** 配置中不存在切换回 Full 产品的模式

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

### Requirement: 目标构建图排除非交付应用
根级 build、typecheck、test、lint、`make check` 和 required CI SHALL 只覆盖 Web 与目标共享包，不得构建或验收 Desktop、Mobile 或 Docs。Desktop 专属源码在本变更中 SHALL 保持未改造状态，且不得以兼容 Desktop 为由保留旧 API。

#### Scenario: 执行根级前端命令
- **WHEN** 在 Fresh Checkout 执行根级 build、typecheck、test 和 lint
- **THEN** 只运行 Web 与目标共享包任务
- **THEN** Desktop、Mobile 和 Docs 均不在执行图中

#### Scenario: 共享包发生变更
- **WHEN** Core、View 或 UI 共享代码发生修改并触发 required CI
- **THEN** CI 验证目标 Web 工作集
- **THEN** Desktop 构建失败不构成本变更的发布门禁

### Requirement: 不提供旧契约兼容
Server、Web 与 Daemon/CLI MUST 同批切换到 Lightweight 契约，不得保留旧 API alias、旧 Desktop 适配器、旧数据库升级器或原完整产品数据库运行模式。

#### Scenario: 旧客户端调用
- **WHEN** 旧 Desktop 或旧 CLI 调用未列入 Lightweight manifest 的路径或字段
- **THEN** 请求被拒绝而不是被兼容层转换

#### Scenario: 新客户端运行
- **WHEN** 目标 Web 与 Daemon/CLI 对同一 Server 运行
- **THEN** 三端使用同一冻结 manifest 和 DTO 契约完成 P0 流程

### Requirement: 范围变更必须显式立项
本变更未列入的能力 MUST NOT 在实现中隐式恢复；新增产品范围、兼容要求或量化阈值调整 SHALL 通过独立 OpenSpec change 审批。

#### Scenario: 实现需要恢复已退出能力
- **WHEN** 某实现任务提出重新启用 Cloud、Attachment、Invitation、Desktop 兼容或其他退出能力
- **THEN** 当前变更停止吸收该范围，并要求建立独立 change

### Requirement: Server-side MCP Gateway 具有受限 Web 管理面
Lightweight 产品 SHALL 允许 Web Server 后端提供 `server-mcp-gateway`、`server-tool-bundle` 与 `server-tool-source-management` 规定的 MCP Data Plane 和 Human Control Plane API，并允许目标 Web 通过 `web-tool-source-management` 与 `web-agent-tool-bundle-management` 暴露受权限保护的管理入口。目标 Web 的允许表面仅包括 Workspace `Tools` route/Sidebar、Tool Source detail/import flow、Agent `Capabilities > Tools`、经过 schema 校验的共享 API client/query 和对应 i18n。

该例外 MUST NOT 恢复 raw `agent.mcp_config` Reveal/Update UI/API、Runtime-local MCP 管理、Integrations、通用 Attachment、Desktop wiring、Mobile/Docs flow 或其他已退出产品面。Web MUST NOT 包含 Gateway protocol、registry、parser/compiler、HTTP/gRPC/MCP client、credential、connection pool、egress policy、audit 或 invocation 实现。

全部 Gateway protocol、registry、parser/compiler、HTTP/gRPC/MCP client、credential、connection pool、policy、audit 与 invocation 能力 SHALL 只存在于 Web Server 后端执行图。Local Daemon/CLI SHALL 保持源码、协议和既有 Provider projection 行为不变，且 MUST NOT 新增 Gateway package、Tool Registry、OpenAPI/Proto parser、gRPC client、MCP proxy、tunnel 或 callback。

#### Scenario: 发布 Server Gateway
- **WHEN** 操作者部署包含本变更的 Lightweight Server，并继续使用未修改的 Local Daemon/CLI
- **THEN** Bundle Task 的 Agent Runtime 通过既有 Claim `mcp_config` 接入 Server `/bundles/{bundleId}/mcp`
- **THEN** Server 执行本能力所含 Bundle discovery、authorization 和 upstream invocation，Daemon 不理解或执行 Gateway 内部类型

#### Scenario: Daemon 保留 runtime-local MCP
- **WHEN** Daemon 在本机 Runtime 配置中发现不属于 Server ToolBundle 的 MCP entries
- **THEN** Daemon 按既有逻辑继续合并和投影这些 entries
- **THEN** 本变更不要求 Server 能看到、代理、禁用或保证这些本机 MCP 的行为

#### Scenario: 扫描目标产品表面与依赖图
- **WHEN** 发布流程扫描 Web routes、Sidebar、Agent tabs、shared API client、Desktop/Mobile/Docs、Daemon/CLI imports 与二进制依赖图
- **THEN** 目标 Web 只包含 Workspace Tools 与 Agent Tools 控制面和脱敏 REST client，不包含 raw MCP config 或 Gateway 执行依赖
- **THEN** Desktop/Mobile/Docs 不增加该入口，Daemon/CLI 不依赖 Gateway/MCP SDK/gRPC/OpenAPI/Proto compiler package，只有 Server target 包含 Gateway 执行能力与依赖

#### Scenario: Web 管理操作不下沉执行能力
- **WHEN** Owner/Admin 在 Web 上传 Proto/OpenAPI artifact、校验 Source 或为 Agent 发布 ToolBundle
- **THEN** Web 只向 Server 提交 artifact/metadata/selection 并展示脱敏结果
- **THEN** 格式识别、转换、凭据处理、上游连接、Bundle 物化和 MCP 调用全部在 Server 完成

### Requirement: Server 不可达网络不扩展到本地执行面
Server-side Gateway SHALL 只支持 Server 可以按 egress policy 直接访问的 REST、gRPC 与 Remote MCP target。Daemon localhost、用户设备 localhost、用户 VPN、局域网或设备凭据依赖 target MUST NOT 通过本变更增加 local gateway、tunnel、reverse proxy、port forwarding 或 Agent direct-access fallback。

#### Scenario: Source 只在 Daemon 网络可达
- **WHEN** Tool Source endpoint 仅存在于 Daemon 主机、用户 VPN 或局域网且 Server 无法直接访问
- **THEN** Source validation 失败并明确归类为 unsupported network topology
- **THEN** 系统不要求修改 Daemon 或让 Agent Runtime 持有 upstream credential 绕过限制
