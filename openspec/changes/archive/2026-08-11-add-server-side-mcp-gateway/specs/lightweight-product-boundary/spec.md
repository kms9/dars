## ADDED Requirements

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
