# MCP Gateway 运维说明

## 能力边界

MCP Gateway 是 DARS Web Server 后端能力。它把已发布的不可变 ToolBundle 投影为 `/bundles/{bundleId}/mcp`，并使用当前执行任务（task）的 `dat_` token 鉴权。守护进程、CLI、Provider adapter 和 wire protocol 不需要修改；守护进程既有的本机 MCP 合并行为保持不变。

ToolBundle 只保证 Server 持久化的工具清单、schema、Source revision、artifact digest 和 invocation plan 不变。它不保证外部 HTTP、gRPC 或 MCP 服务的可达性、可用性、延迟或行为稳定性。自动化验收使用受控本地 fixture，不把结果扩展为外部服务可用性证明。

当前代码拥有的 Provider allowlist 只包含 `codex`。该组合曾使用发布构建的未修改守护进程与 Codex Runtime 0.147.0 完成路径迁移前的真实 E3 兼容验证；最终 `/bundles/{bundleId}/mcp` 路径的 exact-path E3 尚需重跑，因此当前 change 仍有一项发布门禁未完成。其他 Provider 尚未验证，会在 Bundle 发布、运行时变更、执行任务创建和 Claim 阶段 fail closed；新增 Provider 必须先补齐同等 E3 证据再修改代码 allowlist，不能通过部署配置绕过。

## 配置

| 环境变量 | 默认值 | 说明 |
| --- | --- | --- |
| `DARS_PUBLIC_URL` | 空 | 对外可达的绝对 HTTP(S) Server URL。空值允许 Server 启动，但有 ToolBundle pin 的 Claim 会失败。不得包含 userinfo、query 或 fragment。 |
| `DARS_MCP_PRIVATE_CIDR_ALLOWLIST` | 空 | 部署级私网 CIDR 白名单，逗号分隔。默认拒绝全部 private/VPC 地址。 |
| `DARS_MCP_ALLOWED_PORTS` | `80,443` | 允许的上游端口，逗号分隔。 |
| `DARS_MCP_CONNECT_TIMEOUT` | `5s` | DNS/连接超时。 |
| `DARS_MCP_INVOCATION_TIMEOUT` | `30s` | 单次验证或工具调用的总时限。 |
| `DARS_MCP_CHANNEL_IDLE_TIMEOUT` | `5m` | 空闲 gRPC channel 的回收时间。 |
| `DARS_MCP_MAX_REQUEST_BODY_BYTES` | `1048576` | MCP 请求与上游请求上限。 |
| `DARS_MCP_MAX_RESPONSE_BYTES` | `4194304` | 上游响应与 MCP 结果上限。 |
| `DARS_MCP_MAX_ARTIFACT_BYTES` | `16777216` | OpenAPI/Proto/descriptor artifact 及展开后的 archive 上限。 |
| `DARS_MCP_MAX_REDIRECTS` | `3` | HTTP redirect 上限；每一跳重新执行 egress policy。 |
| `DARS_MCP_TASK_CONCURRENCY` | `4` | 每个执行任务的并发工具调用上限。 |
| `DARS_MCP_SOURCE_CONCURRENCY` | `8` | 每个 Source 的并发工具调用上限，也是单副本 Source validation worker 上限。 |

Source credential 使用 `DARS_AGENT_SECRET_KEY` 和 `DARS_AGENT_SECRET_KEY_ID` 的版本化 AEAD envelope。生产环境的 `/readyz` 会检查该密钥；缺失或非法时 readiness 返回错误。配置值格式非法时 Server 启动失败，不会降低为明文存储。

## 网络与 TLS

所有 OpenAPI URL/ref、HTTP、gRPC 和 Remote MCP 连接都在验证和每次 dial 时重新执行统一 egress policy：

- 始终拒绝 userinfo、loopback、link-local、multicast、unspecified 和云 metadata 地址。
- private/VPC 地址只有在部署级 CIDR 白名单中才能访问，不提供工作区级绕过。
- DNS 的全部解析结果必须获准，实际连接固定到已批准 IP；redirect 会重新验证目标。
- HTTP 客户端不读取环境 proxy。跨 authority redirect 不携带 Source credential。
- HTTPS/gRPC TLS 使用原始 hostname 校验证书，最低 TLS 1.2。
- 工具调用不自动重试，避免非幂等副作用重复执行。

只在守护进程主机、用户 VPN、localhost 或局域网可达的服务不受支持。此能力不会创建 tunnel、local gateway、reverse proxy、port forwarding，也不会把上游 credential 下发给智能体运行时作为 fallback。

## 诊断与观测

- `GET /health` 只表示进程存活；`GET /readyz` 检查数据库、schema、生产邮件和加密密钥 readiness。
- 设置 `METRICS_ADDR` 后，可采集 `dars_mcp_gateway_*` 指标。指标 label 只有 Source kind 和有界 outcome，不包含工作区、智能体、task、Bundle、Source、tool 或凭据 identity。
- `activity_log` 记录 Source validation、Bundle publication/revoke 和 tool invocation 的身份、结果、延迟与请求/响应尺寸。它不记录参数、结果、headers、token、artifact、descriptor 或 credential 内容。
- `egress_forbidden` 表示目标被网络策略拒绝；`tool_source_unreachable` 表示受控连接失败；`provider_mcp_unsupported` 表示 Provider 尚未通过兼容矩阵。

排查 Claim 失败时，先确认执行任务已经固定 Bundle、Bundle/Source 未撤销、Provider 已进入代码 allowlist，再确认 `DARS_PUBLIC_URL` 是 Runtime 可访问的完整外部地址。不要从请求 Host 或 forwarded headers 推导 Gateway URL。

## 撤权与回滚

回滚不需要修改或迁移守护进程：

1. 清除 Agent current Bundle head，阻止新执行任务获得 Gateway 能力。
2. 撤销活跃 Bundle，或禁用相关 Source，立即阻止新的 `tools/list` 和 `tools/call`。
3. 等待在途调用达到 `DARS_MCP_INVOCATION_TIMEOUT`；Server shutdown 会取消在途 Gateway 请求并关闭 replica-local gRPC channel。
4. 部署上一版 Server。旧 Server 会忽略新增表；不要在回滚过程中删除 retained Bundle 引用的 artifact 或 Secret。

数据库 schema 删除属于独立的破坏性变更，不是本回滚流程的一部分。
