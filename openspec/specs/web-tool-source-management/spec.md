# web-tool-source-management Specification

## Purpose
定义目标 Web 为 Workspace Owner/Admin 提供的 Tool Source 上传、管理、校验与 tool catalog 预览入口，同时保证浏览器只承担控制面交互，不获得 Server Gateway 执行能力或上游 Secret。
## Requirements
### Requirement: Workspace 提供受权限保护的 Tools 管理入口
目标 Web SHALL 在 Workspace Sidebar 提供 `Tools` 入口，并提供 `/{workspaceSlug}/tools` 与 `/{workspaceSlug}/tools/{sourceId}` 页面。入口和操作 SHALL 仅对 Workspace Owner/Admin 可见；普通 Member 直接访问 route 或调用 API 时 SHALL 得到一致的无权限结果。

列表 SHALL 展示 Source name、kind、enabled/status、current revision、tool count、updated time 与脱敏 validation summary，并提供创建、刷新和进入详情操作。页面 MUST NOT 显示 credential、artifact bytes、原始 auth headers 或 parser/network internals。

#### Scenario: Admin 打开 Tools 页面
- **WHEN** Workspace Owner/Admin 打开 Sidebar Tools
- **THEN** 页面从 Server 加载当前 Workspace Tool Sources，并按脱敏状态呈现可操作目录

#### Scenario: Member 直接访问 Tools
- **WHEN** 普通 Member 通过导航或直接 URL/API 尝试访问 Tools 管理面
- **THEN** 导航不显示该入口且请求返回 403/404，不泄露 Source 是否存在

### Requirement: Web 提供分类型导入流程
Tools 页面 SHALL 提供导入流程并明确支持矩阵：OpenAPI 3.x JSON/YAML 文件或 URL、Swagger 2.0 JSON/YAML 文件或 URL、单个 `.proto`、bounded Proto ZIP、descriptor set，以及 HTTP(S) Streamable Remote MCP endpoint。`server_local` Source MUST NOT 作为用户可创建类型。

上传型格式 SHALL 使用 Server multipart 原子导入；URL 型格式 SHALL 使用 Server strict JSON contract。Web SHALL 显示所选文件名、大小、格式选项、校验进度、稳定 validation error 和已发现 tools 预览，但 MUST NOT 在浏览器解析/编译 Swagger/OpenAPI/Proto 或直接连接 upstream。

#### Scenario: 上传 Proto 包
- **WHEN** Admin 选择 gRPC、上传受支持 artifact 并填写 Server 可达 endpoint
- **THEN** Web 将文件和 metadata 提交给 Server，并在成功后显示 unary tools；streaming 或无效依赖显示稳定错误且不产生半创建 Source

#### Scenario: 导入 Swagger URL
- **WHEN** Admin 提交经 Server egress policy 允许的 Swagger/OpenAPI URL
- **THEN** Server 获取、识别、转换和校验 document，Web 只显示 redacted status、validation error 或 catalog；不能通过统一编译的转换结果不得发布

### Requirement: Source 详情支持生命周期管理
Source detail SHALL 展示 current/staged revision、发现的 Tools、endpoint redacted metadata、artifact metadata、validation status/error 和 enabled 状态，并提供 validate、enable、disable、更新与删除操作。导入表单 SHALL 将 Source name 标记为 canonical namespace；发现的 Tool SHALL 分别显示只读 canonical MCP name 与 upstream operation，且 Source detail MUST NOT 提供修改已发布 Definition canonical identity 的控件。Source revision 更新 MUST NOT 自动更新任何 Agent Bundle；Web SHALL 标记使用旧 revision 的 Agent selection 为 `update available` 并要求主动重新发布。

#### Scenario: Source 发布新 revision
- **WHEN** Admin 成功更新一个已被 Agent Bundle 使用的 Source
- **THEN** Source detail 显示新 catalog，旧 Agent Bundle 保持不变，并在 Agent Tools 页面显示可主动更新的提示

#### Scenario: 禁用 Source
- **WHEN** Admin 从详情页禁用 Source
- **THEN** 页面刷新为 disabled，Server 对受影响 Bundle 后续 discovery/call 立即 fail closed

#### Scenario: 用户需要自定义 Agent 调用名
- **WHEN** Admin 在 Source detail 查看一个已发布 Tool
- **THEN** 页面说明 canonical MCP name 由导入 revision 固定，并引导在 Agent `Capabilities > Tools` 配置 Agent-specific exported name

