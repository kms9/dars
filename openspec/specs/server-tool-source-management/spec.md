# server-tool-source-management Specification

## Purpose
定义 Workspace 管理员在 Server 后端注册、验证、修订和停用企业 Tool Sources，并把选定 Definitions 发布为 Agent ToolBundle 的控制面，以及 OpenAPI、动态 gRPC 与 Remote MCP 被规范化为统一 Tool Definitions 时必须满足的安全和生命周期约束。
## Requirements
### Requirement: Tool Source Control Plane 仅由授权 Human 管理
Server SHALL 提供 Workspace-scoped Tool Source、Tool Definition 与 Agent ToolBundle publication 的 Control Plane API。Workspace Owner/Admin SHALL 能创建、验证、读取 redacted metadata、修订、启用、禁用和删除 Source，并为 Workspace 内 Agent 发布/读取不可变 Bundle 或执行显式撤权；普通 Member、Task Token、Daemon Token 与跨 Workspace Human MUST NOT 管理 Source/Bundle 或 Reveal Source credential。

目标 Web MAY 通过受权限保护的 Tool Source 页面和经过 schema 校验的 Control Plane client 消费这些 API。Server MUST NOT 提供 raw `agent.mcp_config` Reveal/Update API、credential reveal 或通用 Integration/Attachment 产品面。

#### Scenario: Admin 管理 Tool Source
- **WHEN** Workspace Admin 创建 Source、查看验证状态并为同 Workspace Agent 发布工具选择
- **THEN** Server 完成操作并只返回 endpoint/policy/revision/status、Bundle ID/hash/summary 等 redacted metadata

#### Scenario: 非授权身份管理 Source
- **WHEN** Member、Task Token、Daemon Token 或其他 Workspace Admin 尝试创建、更新、发布 Bundle、删除或 Reveal Source
- **THEN** Server 返回 403/404，且不泄露 Source、credential、artifact 或 validation detail

### Requirement: Tool Source 具有原子修订生命周期
每个 Tool Source SHALL 具有 Workspace identity、kind、stable name、revision、enabled 状态和 `validating/ready/failed/disabled` 生命周期。Source create/update SHALL 先在有界环境验证 endpoint、artifact、credential shape 与 generated definitions；只有完整 revision 验证成功后，Server 才能原子发布该 revision。失败 revision MUST NOT 替换上一个 ready revision。

Tool Definition SHALL 具有稳定、确定且 Workspace 内无冲突的 public name，并记录 input/output schema、source revision 与不可由调用参数覆盖的 operation metadata。Source 新 revision SHALL 只影响后续 Bundle publication，不得改写已发布 Bundle；Source emergency disable SHALL 立即阻止受影响 Bundle 的新 discovery/call。Source delete SHALL 尊重 retained Bundle/artifact 引用，并按应用层事务拒绝或显式清理 definitions、artifacts、secrets、Bundle references 与 connection/cache state。

#### Scenario: 成功发布新 revision
- **WHEN** Admin 更新 ready Source 且新 artifact、endpoint 和 definitions 全部验证成功
- **THEN** Server 原子切换 Catalog current revision，后续新 Bundle 只选择完整的新 definitions
- **THEN** 已发布 Bundle 仍固定旧 revision，除非被显式撤权

#### Scenario: 新 revision 验证失败
- **WHEN** 更新包含无法解析的 schema、缺失依赖、名称冲突或不允许的 endpoint
- **THEN** 新 revision 标记 failed 并返回 redacted validation error，上一个 ready revision 继续保持一致可用

### Requirement: Source catalog 名称与 Agent 调用别名分离
上传或创建 Source 时的 stable name SHALL 作为生成 Tool Definition canonical public name 的 namespace。Definition canonical public name SHALL 由 Source namespace 与稳定 upstream operation key 确定，并随 Source revision 不可变；已发布 Definition MUST NOT 通过详情页、Control Plane update 或 Bundle publication 被原地重命名。

Control Plane SHALL 把 canonical public name 与 upstream name 作为不同字段返回。Agent-specific exported name MUST NOT 写回 Tool Source、Source revision 或 Tool Definition；它只属于新发布的不可变 Bundle item。

#### Scenario: 上传后查看生成名称
- **WHEN** Admin 以 Source namespace `kratos-demo-http` 导入 upstream operation `User_GetUser`
- **THEN** catalog 显示确定的 canonical public name `kratos-demo-http.User_GetUser` 和独立 upstream name
- **THEN** Source detail 不提供修改该已发布 canonical identity 的操作

#### Scenario: 两个 Agent 使用不同调用名
- **WHEN** Agent A 和 Agent B 选择同一 Tool Definition 但分别配置不同 exported name
- **THEN** Source/Definition 保持同一 canonical identity，两个 Agent 获得各自独立的新 ToolBundle

### Requirement: OpenAPI Source 预编译为受限 HTTP tools
OpenAPI Source SHALL 接受有大小上限的 OpenAPI 3.x JSON/YAML document、Swagger 2.0 JSON/YAML document 或经 egress policy 允许的 URL。Swagger 2.0 SHALL 在 Server 内转换为规范化 OpenAPI 3 contract 后再进入同一编译流程；Web MUST NOT 自行转换或执行 document。Server SHALL 在发布 revision 前识别格式和版本、解析 document、解析受限 refs、验证 operation、parameters、request body、response 与 auth mapping，并预编译为 Tool Definitions；每次 `tools/call` MUST NOT 重新解析完整 document。

生成的 HTTP invocation MUST 只使用 Source revision 固定的 scheme/host/base path/method/operation，并对 path/query/header/body arguments 做 schema validation。调用参数 MUST NOT 注入 Host、Authorization、hop-by-hop headers、任意 URL、redirect policy 或 server-held credential。

#### Scenario: 导入合法 OpenAPI
- **WHEN** Admin 导入受支持且 operations 命名无冲突的 OpenAPI document
- **THEN** Server 发布 ready Source 和稳定 Tool Definitions，选入 Bundle 后由 Server 按固定 plan 调用已验证 operation

#### Scenario: 导入 Swagger 2.0
- **WHEN** Admin 上传合法 Swagger 2.0 JSON/YAML document
- **THEN** Server 在有界环境完成版本识别与规范化转换；只有能够通过统一 OpenAPI 3 校验和编译的结果才可发布，不能确认无损映射时返回稳定、脱敏的 validation error 并且不产生部分 revision
- **THEN** 只有转换后完整通过校验的 operations 才能原子发布为 ready definitions

#### Scenario: OpenAPI 尝试越过目标边界
- **WHEN** document、remote ref、redirect 或 tool arguments 尝试访问未批准 host、private address、metadata endpoint 或任意 URL
- **THEN** Server 拒绝 import 或 invocation，且不返回被探测网络的信息

### Requirement: Build-time 与 uploaded Proto 共用受限 dynamic gRPC tools
Server SHALL 将 build-time embedded descriptor set 与 Control Plane 上传的 `.proto`、bounded archive 或 descriptor set 编译/规范化为同一种 descriptor registry 和 Tool Definition。MVP SHALL 只发布可由单次 MCP `tools/call` 完整表达的 unary RPC；client-streaming、server-streaming、bidirectional-streaming、无法解析依赖或 schema 无法安全映射的 method MUST fail validation。

Dynamic invocation SHALL 根据已发布 Method Descriptor 创建 request/response、严格执行 ProtoJSON mapping，并通过 Server-owned gRPC channel 调用固定 full method name。每个 Server replica MAY 维护本地 channel pool，但数据库 revision SHALL 是定义与授权真相，连接、TLS 和 metadata MUST NOT 来自 Agent arguments。

当这些 tools 被选入 Bundle 时，Bundle SHALL 固定 descriptor artifact ID/digest、Source revision、method 与 schema。只要 retained Bundle 仍引用 artifact，Source update/delete 或后续同名 Proto upload MUST NOT 就地替换或提前删除该 artifact。

#### Scenario: 两种 Proto 来源产生相同调用语义
- **WHEN** 相同 unary service descriptor 分别来自 build-time embed 与 runtime upload
- **THEN** 两者生成等价 schema 并经同一授权、dynamic invocation、deadline、TLS 和 audit contract 执行

#### Scenario: 上传不支持的 Proto
- **WHEN** archive 超限、import 缺失、descriptor 冲突或 service 只包含 streaming RPC
- **THEN** Server 将 revision 标记 failed，且不生成部分可见 definitions 或执行编译产物

### Requirement: Remote MCP Source 仅聚合受支持的远程 tools
Remote MCP Source SHALL 只连接经 egress policy 验证的 HTTP(S) Streamable MCP endpoint，并在发布 revision 前完成协议协商与 tool discovery。Server SHALL 只聚合 tools，不转发 prompts、resources、roots、sampling、logging、任意 server-to-client capability 或上游 credential。

每次 proxied `tools/call` SHALL 在 Server 内建立或取得受控 upstream client context，调用 registry 固定的 upstream tool name，并在完成、失败或取消后释放有界资源。依赖跨调用协议 Session、客户端本地能力或未批准 callback 的 upstream tool MUST NOT 发布为 ready definition。

#### Scenario: 聚合 stateless Remote MCP tools
- **WHEN** Admin 注册可达、获准且能发现 tools 的 Remote MCP endpoint
- **THEN** Server 以 namespace-safe public names 发布 definitions，选入 Bundle 后代理该 Bundle 固定 upstream tool 的调用

#### Scenario: Upstream 要求未支持能力
- **WHEN** upstream tool 依赖 roots、sampling、跨调用 session、任意 callback 或其他未批准客户端能力
- **THEN** Server 拒绝发布或调用该 tool，且不把请求转发给 Agent Runtime/Daemon 代办

### Requirement: Egress policy 在创建和调用时同时执行
所有 OpenAPI URL/ref、HTTP target、gRPC endpoint 与 Remote MCP endpoint SHALL 在 Source validation 和每次 connect/dial 时执行统一 egress policy。Policy MUST 校验 scheme、host、port、DNS 解析结果、redirect、proxy、TLS 与目标 IP；默认 MUST 拒绝 loopback、link-local、multicast、unspecified、云 metadata 和其他未批准网段。Private/VPC target 只有在部署级明确 allowlist 后才能使用，DNS rebinding、redirect 和重新解析 MUST NOT 绕过该 allowlist。

Server 无法直接访问的 Daemon localhost、用户 VPN 或局域网服务 SHALL 被视为 unsupported；系统 MUST NOT 创建 Daemon tunnel、local gateway、reverse callback 或让 Agent 自行访问 credential-bearing upstream。

#### Scenario: 批准 VPC 服务
- **WHEN** endpoint 的全部解析地址、port、TLS identity 与 network range 均被部署 policy 明确允许
- **THEN** validation 与 invocation 可从 Server 建立受控连接

#### Scenario: DNS 或 redirect 改指受限地址
- **WHEN** 创建后 DNS 重新解析或 HTTP redirect 将目标改为 loopback、metadata、未批准 private IP 或其他 host
- **THEN** connect/dial 层阻止请求，且 audit 只记录 redacted policy reason

### Requirement: Upstream credentials 始终由 Server 隔离
Source auth、HTTP headers、gRPC metadata、TLS material 与 Remote MCP credential SHALL 以版本化 authenticated encryption envelope 存储，只能在授权 validation/invocation path 解密。普通 API、MCP discovery/result、Claim payload、Agent Runtime、Daemon、日志、metrics、Realtime、validation error 与 audit MUST NOT 包含 upstream credential 明文。

#### Scenario: 使用 Server-held credential
- **WHEN** 已授权 tool 调用需要 upstream Bearer header、gRPC metadata 或 mTLS material
- **THEN** Server 在受控 outbound client 中应用 credential，Agent Runtime 只持有自己的 task-scoped `dat_` token

#### Scenario: 检查持久化和观测数据
- **WHEN** 操作者检查数据库、备份、API、MCP payload、日志、metrics、Realtime 与 audit
- **THEN** upstream credential 仅以密文或 redacted metadata 出现，无法从这些输出恢复明文

### Requirement: Control Plane 提供页面友好的原子导入契约
Server SHALL 为上传型 Tool Source 提供 bounded `multipart/form-data` 导入，使 Source metadata 与 OpenAPI/Swagger document、单个 `.proto`、Proto archive 或 descriptor set artifact 在一个受控操作中完成创建、存储、格式识别、首个 revision staging 与校验。请求失败、取消或格式不支持时 MUST NOT 留下页面可见的半创建 Source、孤立 Artifact 或部分 Definitions。

URL 型 OpenAPI 与 Remote MCP Source MAY 继续使用 strict JSON 创建/更新契约。上传响应 SHALL 返回 Source、revision/status、artifact redacted metadata 与已发现 Definition summary；失败响应 SHALL 返回稳定、脱敏的 validation code。响应 MUST NOT 返回 artifact bytes、parser internals 或 credential。

#### Scenario: 上传过程中失败
- **WHEN** Admin 上传超限、损坏或依赖不完整的 Proto/OpenAPI artifact，或请求在原子提交前取消
- **THEN** Server 返回稳定脱敏错误且不暴露部分 Source、Artifact 或 Definitions

#### Scenario: 页面导入后立即预览
- **WHEN** Admin 成功导入支持的 artifact
- **THEN** Web 可以用响应或后续读取 API 显示 revision 状态、validation error 和完整发现的 tool catalog，无需读取 artifact 内容
