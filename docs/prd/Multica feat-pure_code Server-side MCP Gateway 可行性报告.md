# Multica `feat/pure_code` Server-side MCP Gateway 可行性报告

## 1. 核心结论

目标架构应该明确为：

```text
                    Multica Web / Control Plane
                              │
                    Tool Source 管理页面
                              │
                              ▼
┌──────────────────── Multica Web Server ─────────────────────┐
│                                                             │
│                  Remote MCP Endpoint                        │
│                     POST /mcp                              │
│                          │                                  │
│                    Tool Registry                            │
│                          │                                  │
│          ┌───────────────┼────────────────┐                 │
│          │               │                │                 │
│          ▼               ▼                ▼                 │
│   OpenAPI Adapter    gRPC Adapter     MCP Proxy Adapter      │
│          │               │                │                 │
│      HTTP Client     grpc.ClientConn    MCP Client           │
│          │               │                │                 │
└──────────┼───────────────┼────────────────┼─────────────────┘
           │               │                │
           ▼               ▼                ▼
      REST Service     gRPC Service     Remote MCP Server


Local Machine
┌─────────────────────────────────────────────────────────────┐
│ Multica Daemon                                              │
│      │                                                      │
│      └── 启动 Codex / Claude / Cursor / ...                 │
│                        │                                    │
│                        │ Streamable HTTP MCP                │
│                        ▼                                    │
│              https://multica-server/mcp                     │
└─────────────────────────────────────────────────────────────┘
```

也就是说：

> **Multica Web Server 同时承担 MCP Server + Tool Gateway。**

Daemon 不承担：

```text
OpenAPI 解析
Proto 编译
gRPC 调用
HTTP API 调用
Remote MCP Proxy
Tool Registry
grpc.ClientConn
```

这些全部属于 Server。

Daemon 只继续承担原来的：

```text
Task Claim
→ 获取 Agent 配置
→ 启动 Agent Runtime
→ 把 Remote MCP 配置交给 Runtime
```

---

# 2. 这个方案与当前 `feat/pure_code` 是匹配的

当前 Lightweight Schema 中已经保留：

```text
agent.mcp_config
```

同时已经存在：

```text
task_token
├── task_id
├── agent_id
├── workspace_id
├── user_id
└── expires_at
```

并且任务表里还有：

```text
runtime_mcp_overlay
```

这些都给 Server-side Remote MCP 留出了不错的接入条件。

当前 Daemon Claim 的 `AgentData` 已经包含：

```text
McpConfig json.RawMessage
```

因此 Server 可以继续通过当前 Task Claim 链路，把 Multica 自己的 Remote MCP 配置交给 Daemon。

更重要的是，当前 Task Claim 本身已经包含由 Server 签发的 **task-scoped AuthToken**：

```text
Task
 ├── AgentID
 ├── WorkspaceID
 └── AuthToken
```

代码明确规定它是 Server mint 的 task-scoped credential，而不是 Daemon 自己的长期凭证。

所以整个基础设施已经具备：

```text
Server
   ↓
Task-scoped token
   ↓
Daemon
   ↓
Agent Runtime
```

我们只需要把它进一步用于：

```text
Agent Runtime
   ↓ Authorization: Bearer task-token
Multica /mcp
```

---

# 3. Daemon 是否需要修改

## 结论：理论上可以做到几乎不修改 Daemon

当前 Daemon 已经能够接收 `McpConfig`，并将 Agent MCP 配置与 Runtime MCP 配置合并。

当前代码也已经把：

```text
url
```

识别为 HTTP 类型 MCP，Codex 的配置差异还会由现有适配层处理。

因此 Server 在 Claim Task 时只需要让 Agent 获得类似：

```json
{
  "mcpServers": {
    "multica": {
      "type": "http",
      "url": "https://multica.example.com/mcp",
      "headers": {
        "Authorization": "Bearer <task-token>"
      }
    }
  }
}
```

后面的链路仍然是：

```text
Task Claim
   ↓
AgentData.McpConfig
   ↓
Daemon
   ↓
effectiveMcpConfig
   ↓
Codex / Claude / Cursor
   ↓
Multica Remote MCP
```

Daemon 根本不需要知道：

```text
student.getProfile
```

实际背后究竟调用：

```text
REST
gRPC
还是另一个 MCP
```

这是非常重要的架构隔离。

---

# 4. Multica Server 应该直接提供 `/mcp`

当前 `feat/pure_code` 的冻结 Route Contract 中还没有 `/mcp`，所以这是一个明确的新 Server Data Plane Endpoint。

推荐：

```text
POST /mcp
```

而不是：

```text
/api/agents/{id}/mcp
/api/tools/mcp
/api/workspaces/{id}/mcp
```

原因是可以把 Multica 本身看作一个统一的 MCP Server：

```text
https://multica.xxx.com/mcp
```

Tool Set 根据当前认证身份动态计算。

最新 MCP `2026-07-28` Streamable HTTP 本身就是单个 HTTP POST endpoint，每个 JSON-RPC 请求通过独立 POST 完成，并已经移除了协议级 Session。

这非常适合 Multica Web Server。

---

# 5. 推荐直接使用官方 Go MCP SDK

不建议自己写：

```text
JSON-RPC
initialize / discover
tools/list
tools/call
protocol negotiation
Streamable HTTP
SSE compatibility
```

官方 Go MCP SDK 已经提供：

```text
mcp.Server
mcp.Client

StreamableHTTPHandler
StreamableServerTransport
StreamableClientTransport
```

其中 `StreamableHTTPHandler` 本身就是一个 Go `http.Handler`，可以直接挂到现有 HTTP Server/Chi Router。

截至 2026 年 8 月，官方 Go SDK v1.7.0 已支持 `2026-07-28` MCP，并保持对旧协议的兼容。对于 `2026-07-28` Streamable HTTP，SDK 要求采用 Stateless 模式。

所以推荐：

```text
Existing Multica Go Server
        │
        ├── /api/*
        │
        └── /mcp
              │
      StreamableHTTPHandler
```

不是再启动一个独立 MCP 进程。

---

# 6. 为什么 Stateless MCP 特别适合这个架构

推荐 Multica MCP 使用：

```text
StreamableHTTPOptions{
    Stateless: true
}
```

因为最终部署通常会变成：

```text
                Load Balancer
                     │
         ┌───────────┼───────────┐
         ▼           ▼           ▼
     Server 1    Server 2    Server 3
```

Agent 每一次：

```text
tools/list
tools/call
```

都可以进入不同 Server Instance。

不需要：

```text
MCP sticky session
MCP session store
Daemon affinity
Agent → 固定 Server
```

2026-07-28 MCP Transport 已经转向这种单请求、无协议 Session 的模式。

这与 Web Backend 做 MCP Gateway 比 Local Daemon Gateway 更自然。

---

# 7. Agent Runtime 到 Multica MCP 的鉴权

这里推荐直接复用：

> **Task Token**

而不是给 Agent 创建一个永久 MCP API Key。

当前已经存在：

```text
task_token

task_id
agent_id
workspace_id
user_id
expires_at
```



因此：

```text
Agent Runtime
       │
       │ Authorization:
       │ Bearer mat_xxxxx
       ▼
POST /mcp
       │
       ▼
MCP Auth Middleware
       │
       ├─ workspace_id
       ├─ agent_id
       ├─ task_id
       └─ user_id
```

然后：

### tools/list

```text
task-token
   ↓
AgentID
   ↓
AgentToolBindings
   ↓
只返回 Agent 有权限的 tools
```

### tools/call

再次检查：

```text
token.workspace
token.agent
tool.workspace
agent-tool binding
tool enabled
```

全部通过后才允许执行。

这样天然符合当前 Multica：

```text
Workspace
Agent
Task
Invocation Permission
```

的安全模型。

---

# 8. MCP 配置不要永久保存 Task Token

这里需要和原来的 `agent.mcp_config` 区分。

不建议 DB 永久存：

```text
Authorization: Bearer xxx
```

正确方式应该是：

```text
Agent.mcp_config
       +
Task Claim 时生成
Multica MCP Overlay
       ↓
最终 McpConfig
```

概念上：

```text
Stored Agent MCP

github MCP
database MCP
...

        +

Runtime Generated MCP

multica:
  url = SERVER_PUBLIC_URL + "/mcp"
  auth = 当前 task token

        ↓

effective task MCP config
```

当前 Schema 已经存在 `runtime_mcp_overlay` 字段。

**建议优先评估能否复用这个字段。**

不过这一点我会标记为：

> 需要实现阶段进一步确认现有 `runtime_mcp_overlay` 的完整生产消费链路。

因为本轮代码检查确认了 Schema 存在，但没有完整确认它当前是否已经覆盖你需要的所有 Provider MCP 渲染路径。

即使不能直接复用，也应该在 **Server Task Claim 阶段动态生成 McpConfig**，而不是改成 Daemon Gateway。

---

# 9. 能力一：Swagger / OpenAPI → Multica Remote MCP

## 数据流

```text
Web UI
  │
  │ swagger.json / openapi.yaml / URL
  ▼
Multica Server
  │
  ├─ Parse
  ├─ Resolve refs
  ├─ Validate
  ├─ Generate Tool Definitions
  └─ Store
        │
        ▼
Tool Registry
```

Agent 调用：

```text
Agent Runtime

tools/call
school.getStudent

      ↓ HTTPS

Multica /mcp

      ↓

OpenAPI Tool Adapter

      ↓

HTTP Client

      ↓

Business REST API
```

这里 HTTP 请求发起方明确是：

> **Multica Web Server。**

OpenAPI 本身已经明确描述了 operation、parameters、requestBody、responses 等结构，因此可以在导入阶段预编译成 MCP Tool Definition，而不需要每次调用重新解析 Swagger。

---

# 10. 能力二：编译期 Proto → gRPC → MCP

这里“编译期”也应该明确成：

> **Web Server 编译期。**

而不是 Daemon 编译期。

推荐：

```text
repo/proto/*.proto
       │
       ▼
Build Stage
       │
       ▼
FileDescriptorSet
       │
       ▼
go:embed
       │
       ▼
Multica Server Binary
```

Server 启动：

```text
Embedded FileDescriptorSet
       ↓
Descriptor Registry
       ↓
gRPC ToolDefinitions
       ↓
Tool Registry
```

Protobuf 官方支持通过 compiler 输出 `FileDescriptorSet` 来描述一组 proto 文件。

因此编译期 Proto 和后面的在线 Proto 可以共享同一个 gRPC Runtime。

---

# 11. grpc.ClientConn 现在应该在哪里

这里重新冻结结论：

> **grpc.ClientConn 放在 Multica Web Server。**

结构：

```text
Agent Runtime
      │
      │ MCP / HTTPS
      ▼
Multica Server
      │
      │ grpc.ClientConn
      ▼
StudentService
CourseService
WrongQuestionService
...
```

Server 内部：

```text
GRPCClientManager

source A
  └── ClientConn A

source B
  └── ClientConn B

source C
  └── ClientConn C
```

每一个 Web Server Replica 自己维护自己的 gRPC ClientConn Pool。

gRPC-Go 当前推荐通过 `grpc.NewClient` 创建 `ClientConn`；`ClientConn` 本身代表的是一个到逻辑 endpoint 的虚拟 channel，可以负责解析、连接建立、重试和负载均衡，而不是“一次 RPC 一个 TCP 连接”。

所以：

```text
tools/call
   ↓
不要 Dial
   ↓
直接复用 ClientConn.Invoke
```

---

# 12. gRPC 不需要生成客户端代码

在线 Proto 能力成立的关键仍然不变：

```text
MethodDescriptor
       ↓
dynamicpb
       ↓
grpc.ClientConn.Invoke
```

Go Protobuf 的 `dynamicpb` 可以直接根据 runtime descriptor 创建 message。

所以：

```text
MCP JSON arguments

      ↓

protojson / dynamicpb

      ↓

Dynamic Request

      ↓

grpc.ClientConn.Invoke(
    "/student.StudentService/GetStudent"
)

      ↓

Dynamic Response

      ↓

ProtoJSON

      ↓

MCP Result
```

没有必要：

```text
上传 proto
→ 生成 pb.go
→ go build
→ 重启 Server
```

---

# 13. 能力三：页面上传 Proto → gRPC → MCP

这个需求与能力二应该完全共享 gRPC Runtime。

区别只在 Descriptor 来源：

```text
           gRPC Tool Runtime

                 ▲
                 │
       FileDescriptorSet
         ▲               ▲
         │               │
   Build-time          Runtime
     embed             upload
         │               │
      Proto A         Proto B
```

在线流程：

```text
Web Upload

.proto
proto.zip
.protoset

      ↓

Multica Server

      ↓

protocompile

      ↓

FileDescriptorSet

      ↓

Descriptor Registry

      ↓

Tool Definitions
```

`protocompile.Compiler` 可以在 Go Runtime 中将 Proto Source 编译成 descriptors，并不要求为这些在线 Proto 生成 Go Stub。

因此这是非常适合 Web Server 动态导入的方案。

---

# 14. 能力四：其他 HTTP MCP → Multica MCP

现在也不再是：

```text
Remote MCP
→ Local MCP
```

而应该定义成：

> **Remote MCP Aggregation / Proxy**

结构：

```text
Agent Runtime
      │
      │ MCP
      ▼
Multica /mcp
      │
      │ MCP Client
      ▼
External MCP Server
```

因此 Multica Server 同时是：

```text
面向 Agent：
MCP Server

面向 Upstream：
MCP Client
```

官方 Go MCP SDK 已经同时提供 Streamable HTTP Server 与 Client Transport，可以直接用于这个场景。

执行：

```text
Agent tools/list
      ↓
Multica Tool Registry
      ↓
包含 remote MCP tools
      ↓
返回

Agent tools/call
      ↓
Multica
      ↓
找到 source = mcp_remote
      ↓
MCP Client
      ↓
Upstream tools/call
      ↓
Multica
      ↓
Agent
```

这相当于：

> Multica 成为企业所有 Agent Tools 的统一出口。

---

# 15. 三个 Adapter 就够了

四个 Feature 最终仍然只需要：

```text
Tool Registry
      │
      ├── OpenAPIInvoker
      │
      ├── GRPCInvoker
      │
      └── MCPInvoker
```

其中：

```text
build-time proto
runtime uploaded proto
```

共用：

```text
GRPCInvoker
```

所以真正架构是：

```text
                   MCP Server

                      │
                Tool Registry
                      │
       ┌──────────────┼──────────────┐
       ▼              ▼              ▼
   OpenAPI          gRPC         Remote MCP
   Invoker         Invoker         Invoker
       │              │              │
       ▼              ▼              ▼
     HTTP        grpc.ClientConn    MCP Client
```

---

# 16. 推荐 Server 代码边界

应该从之前的：

```text
server/internal/daemon/toolgateway
```

彻底调整为：

```text
server/
└── internal/
    ├── handler/
    │   └── toolsource.go
    │
    ├── service/
    │   └── toolsource/
    │
    └── mcpgateway/
        ├── server.go
        ├── auth.go
        ├── registry.go
        ├── policy.go
        │
        ├── openapi/
        │   ├── parser.go
        │   ├── schema.go
        │   └── invoker.go
        │
        ├── grpc/
        │   ├── descriptor.go
        │   ├── schema.go
        │   ├── connection_pool.go
        │   └── invoker.go
        │
        └── proxy/
            ├── client.go
            └── invoker.go
```

Daemon 不增加这些 package。

---

# 17. 推荐数据模型

至少增加：

```text
tool_source
```

例如：

```text
id
workspace_id

name
kind

kind:
  openapi
  grpc_builtin
  grpc_dynamic
  mcp_remote

endpoint
revision
enabled

artifact_id
auth_config
transport_config

created_at
updated_at
```

以及：

```text
tool_definition

id
source_id

name
description

input_schema
output_schema

operation_key
invocation_metadata

enabled
```

再增加：

```text
agent_tool_source

agent_id
source_id
enabled
```

关系：

```text
Workspace
     │
 Tool Sources
     │
     ├──── Agent A
     │
     ├──── Agent B
     │
     └──── Agent C
```

---

# 18. `tools/list` 不应该返回整个 Workspace 的 Tool

假设一个 Workspace 有：

```text
OpenAPI A   100 tools
gRPC B       80 tools
MCP C        60 tools
```

不能让每个 Agent 都拿：

```text
240 tools
```

推荐：

```text
Task Token
    ↓
agent_id
    ↓
AgentToolBinding
    ↓
Tool Registry Filter
    ↓
tools/list
```

例如：

```text
错题 Agent
→ 12 tools

教研 Agent
→ 20 tools

开发 Agent
→ 35 tools
```

Server 是 Tool 权限决策的唯一主体。

---

# 19. 最重要的安全边界也发生了变化

原来 Local Gateway 的风险主要在本机。

现在所有请求都从 Server 发出，因此 Server 成为了：

> **统一 Egress Gateway。**

必须重点控制：

```text
OpenAPI URL Import
HTTP Tool Target
gRPC Endpoint
Remote MCP Endpoint
```

否则会形成：

```text
Agent
→ Multica Server
→ 任意内部 IP
```

即严重 SSRF / Internal Network Pivot 风险。

因此 Source 创建时必须有：

```text
Endpoint Validation
DNS/IP Policy
Protocol Allowlist
Private Network Policy
Timeout
Response Size
Concurrency Limit
Credential Isolation
Audit Log
```

这是 Server-side 架构中最重要的 P0 安全项。

---

# 20. 一个硬性部署前提

这个方案有一个必须接受的前提：

> **Multica Web Server 必须能够网络访问目标 REST / gRPC / Remote MCP 服务。**

例如：

```text
Multica Server
      │
      ├── student-service:9000 ✓
      ├── wrong-service:9000   ✓
      └── internal-api:8080    ✓
```

如果某个 gRPC 服务：

```text
只存在于用户 Mac localhost
只存在于 Daemon 所在 VPN
只存在于用户本地局域网
```

那么 Server-side Gateway 就无法调用它。

这种情况不是 MCP 转换能解决的，而是：

```text
Server ↔ Service
```

网络连通性问题。

只要你的目标业务服务本身位于 Server 可访问的内网/Kubernetes/VPC，这个架构就是合理的。

---

# 21. 推荐实施顺序

### Phase 0：Remote MCP Endpoint

先让 Multica Server 成为真正 MCP Server：

```text
POST /mcp
Task Token Auth
tools/list
tools/call
Agent Binding
```

然后让 Codex/Claude Runtime 真正通过网络调用：

```text
Agent
→ Multica /mcp
```

这是最重要的基础验证。

### Phase 1：Remote MCP Proxy

增加：

```text
MCPInvoker
```

验证：

```text
Agent
→ Multica MCP
→ Remote MCP
```

这一步协议转换最少。

### Phase 2：编译期 gRPC

增加：

```text
FileDescriptorSet
dynamicpb
grpc.ClientConn Pool
GRPCInvoker
```

验证真正的：

```text
Agent
→ MCP
→ Multica
→ gRPC
```

### Phase 3：OpenAPI

增加：

```text
OpenAPI Parser
HTTPInvoker
Swagger Import UI
```

### Phase 4：在线 Proto

只增加：

```text
Proto Upload
protocompile
Descriptor Store
```

后面的 GRPCInvoker 100% 复用 Phase 2。

---

# 22. 修正后的可行性判断

| 能力 | 可行性 | 主要难点 |
|---|---:|---|
| Multica Server Remote MCP | 9.5/10 | 鉴权、Tool Binding |
| Swagger/OpenAPI → MCP | 9/10 | Schema 映射、Auth、SSRF |
| Build-time Proto → MCP | 9.5/10 | Descriptor → JSON Schema |
| Online Proto → MCP | 9/10 | Proto dependency、Artifact 管理 |
| Remote MCP → Multica MCP | 9.5/10 | Auth、Tool namespace |
| Server-side gRPC Invocation | 9.5/10 | Conn Pool、TLS、Metadata |

整体建议：

> **立项，可行性高。**

最大的架构风险已经不是 MCP 和 gRPC 转换本身，而是：

```text
多租户权限
Server Egress 安全
Credential 管理
Tool Catalog 规模
动态 Source 生命周期
```

---

# 23. 最终应该冻结的架构原则

这次建议明确冻结为：

```text
1. Multica Web Server 是唯一 MCP Gateway。

2. Agent Runtime 通过 Remote Streamable HTTP MCP 调用 Multica。

3. Daemon 不执行 HTTP/gRPC/MCP Proxy，只负责启动 Runtime 和注入 MCP Config。

4. grpc.ClientConn 全部位于 Web Server。

5. Swagger / Proto / Remote MCP 全部在 Web Server 转换成统一 Tool Registry。

6. Build-time Proto 与 Online Proto 共用 Dynamic gRPC Runtime。

7. MCP 入站鉴权优先复用现有 Task Token。

8. Tool 可见范围由 workspace + agent + task context 决定。

9. Upstream REST/gRPC/MCP credential 永远不发送给 Agent Runtime。

10. `/mcp` 是 Data Plane；
    `/api/tool-sources/*` 是 Control Plane。
```

最终系统不是：

```text
Multica Daemon Tool Gateway
```

而是：

> **Multica Server-side MCP Gateway / Enterprise Tool Gateway**

最终调用链应该牢记成一句话：

```text
Agent Runtime
      ↓ MCP/HTTPS
Multica Web Server
      ↓
OpenAPI / gRPC / Remote MCP
      ↓
Existing Business Services
```

这才是与你当前目标一致的架构。