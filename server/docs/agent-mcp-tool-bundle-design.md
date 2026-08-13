# Agent MCP / Tools 封装与 Proto → MCP 设计纪要

> 状态：设计决策已锁定（未编码）  
> 范围：DARS Server API + MCP Gateway + **恢复 MCP 配置页（验证用）**；**不**改 Agent runtime wrapper
> 关联：[`squad-task-call-chain.md`](squad-task-call-chain.md)（Claim / 执行链路）  
> 飞书同步：https://guanghe.feishu.cn/docx/Nxx1dKX8HovWK2xTwBrcKboQnbg

## 一句话结论

在现有 `agent.mcp_config` → Claim 解密下发 → Daemon 写 sidecar 的链路上，增加 **不可变 ToolBundle（唯一 ID）+ DARS MCP Gateway（facade/proxy）**：Gateway 持有上游 secret，Agent 运行时只持 `dat_`，经 Bundle ID 发现并调用 tools。Proto 路径同时支持 **自有 gRPC codegen** 与 **用户上传 proto → Catalog**，经同一 Gateway 暴露。配置面恢复 MCP 页以便端到端验证。

---

## 已锁定决策

| # | 议题 | 决定 |
|---|---|---|
| 1 | Gateway 形态 | **MCP Facade（B）**：按 Bundle ID 做 `tools/list` / `tools/call`，贴合 Tool Gateway |
| 2 | Bundle 生命周期 | **不可变快照**：改配置 = 新 Bundle ID；运行中 task 绑定 Claim 时的 ID |
| 3 | 凭证归属 | **Gateway 持有上游 secret**；Agent 只持 task-scoped `dat_` |
| 4 | Proto 接入 | **两条都要**：自有 gRPC → MCP tools **且** 用户上传 proto → Catalog |
| 5 | 交付范围 | **Server API + Gateway + 恢复 MCP 配置页**（用于验证当前逻辑） |

明确 Non-goal：不改 Claude / Cursor / Codex / Openclaw 等 Agent runtime wrapper / MCP sidecar 写入逻辑；Claim 仍下发标准 `mcpServers` 形态，只是内容改为指向 DARS Gateway。

---

## 现状锚点

```text
配置态                          执行态
────────                        ────────
UI/API 写 agent.mcp_config
  {"mcpServers": {...}}  ──►  Claim.buildClaimedTask
  (加密落库)                    解密 → claim.agent.mcp_config
                                      │
                                      ▼
                               Daemon 按 provider 写 sidecar
                               (--mcp-config / .cursor/mcp.json / ACP …)
                                      │
                                      ▼
                               Agent CLI 直连上游 MCP
```

要点：

- Server 已有加密 `mcp_config`、Claim 解密、Daemon 多 provider sidecar。
- **尚无**「按 ID 解析 / DARS MCP 门面 / proto→tools」层。
- Lightweight 曾收回 MCP 管理面；本决策要求 **恢复 MCP 配置页** 做验证，需同步调整 Lightweight product boundary 相关约束（客户端重新暴露 MCP 配置能力）。

---

## 能力 1：按智能体配置封装出唯一 Tool Bundle ID

### 路径选择（已定 B）

| | A. Config Registry（配置登记） | B. MCP Facade / Tool Gateway（**已选**） |
|---|---|---|
| ID 解析结果 | 返回真实 `mcpServers` JSON | 对外暴露 `tools/list` + `tools/call` |
| Agent 侧 | 仍连上游 MCP | 只连 DARS 一个 MCP endpoint |
| Claim 变化 | 可继续塞整包，或只塞 URL+ID | Claim 里写成「指向 DARS 的 mcpServers」 |
| 改 runtime? | 否 | 否（只要 `mcp_config` 形态不变） |
| Tool 裁剪 | 配置层过滤后下发 | 门面层按 allowlist 暴露 |
| 秘密 / 凭证 | 下发前解密，或下发引用 | **Gateway 持有上游凭证，Agent 只拿 `dat_`** |

数据模型仍用 Catalog + Bundle（A 的寻址语义）；运行时只暴露 Facade（B）。

### 概念模型

```text
┌──────────────────────────────────────────────────────────────┐
│ Workspace Tool Catalog                                        │
│  • Upstream MCP defs (stdio/http/sse + secrets，仅 Gateway 可读)│
│  • ProtoToolPacks（自有 gRPC + 用户上传 proto）                │
└──────────────────────────┬───────────────────────────────────┘
                           │ select + filter tools
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ ToolBundle（不可变快照）                                       │
│  id: tb_01H...                                                │
│  selection: [ {source, tools[]}, ... ]                        │
│  content_hash: sha256(canonical selection)                    │
│  改配置 → 新建 Bundle（新 ID），Agent 指针切到新 ID              │
└──────────────────────────┬───────────────────────────────────┘
                           │ agent 绑定 tool_bundle_id
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ Claim 改写 mcp_config（标准形态，无上游 secret）                │
│ {                                                             │
│   "mcpServers": {                                             │
│     "dars": {                                                 │
│       "url": "https://…/bundles/tb_01H…/mcp",                 │
│       "headers": { "Authorization": "Bearer dat_…" }          │
│     }                                                         │
│   }                                                           │
│ }                                                             │
└──────────────────────────┬───────────────────────────────────┘
                           │ Agent 标准 MCP 客户端
                           ▼
┌──────────────────────────────────────────────────────────────┐
│ DARS MCP Gateway（Facade）                                    │
│  鉴权 dat_ → 校验 task/agent 与 bundle 绑定                   │
│  tools/list  → Bundle allowlist 投影                          │
│  tools/call  → 用 Gateway 侧 secret 路由上游 MCP / gRPC       │
└──────────────────────────────────────────────────────────────┘
```

### ID 命名

| 方案 | 例子 | 角色 |
|---|---|---|
| 权威 ID | `tb_01HXYZ…`（UUID / ULID） | 主键、Claim、Gateway 路径 |
| 展示名 | `a/mcp1+tools1` | UI / 调试 label，不进主键 |
| content_hash | `sha256(canonical selection)` | 同配置去重；命中则可复用已有不可变 Bundle |

```text
不是:  agentA-mcp1-tools1   （把 agent 编进工具身份）
而是:  ToolBundle(selection) ← Agent 引用

同一 Bundle 可被多个 Agent 复用；
Agent 换工具集 = 绑定新 bundle_id（新建不可变快照）。
```

### Claim 时序（零 runtime 改造）

```text
Daemon Claim
    │
    ▼
Server buildClaimedTask
    │  读 agent.tool_bundle_id
    │  取不可变 Bundle 快照（运行期冻结该 ID）
    │  签发 dat_（已有；可绑定 bundle_id 声明）
    │  改写 MCPConfig → 仅 DARS Gateway URL + dat_
    │  （不上送上游 MCP secret / proto 凭据）
    ▼
Daemon 写 sidecar（现有路径不动）
    ▼
Agent CLI → DARS MCP Gateway
    │  tools/list(bundle)
    │  tools/call → Gateway 持 secret 代理
```

### 与现有 `mcp_config` 的关系

权威状态迁移目标：

- **编辑草稿 / UI 表单** 可仍表现为「选 MCP / tools / proto packs」
- **保存时** 物化（或 content_hash 复用）不可变 `ToolBundle`，Agent 写 `tool_bundle_id`
- **Claim** 只下发 Gateway 入口；旧「整包解密 mcp_config 直连上游」退出主路径

迁移可分阶段：兼容层编译旧 `mcp_config` → Bundle，再切 Gateway；验证期以 UI + API 走新路径为准。

### UI：恢复 MCP 配置页

用途：端到端验证 Catalog 选择 → Bundle 物化 → Claim 改写 → Gateway tools 投影。

预期页面能力（验证期最小集）：

1. 浏览 / 管理 Catalog（上游 MCP、ProtoToolPack）
2. 为 Agent 勾选 sources + tools → 保存生成/复用 Bundle
3. 展示当前 `tool_bundle_id`、content_hash、只读快照摘要
4.（可选）试连 Gateway：用调试 token 看 `tools/list` 投影

实现时需放宽 Lightweight 中「退休 MCP view / 禁止 client 暴露 mcp_config」的契约，改为新的 Bundle/Catalog API 面，而不是简单复活旧直连配置字段语义。

---

## 能力 2：Proto → MCP Tools（双路径都要）

对标：`gRPC Tool Gateway`、Redpanda `protoc-gen-go-mcp`。  
与能力 1 正交，共享 Facade Gateway。

```text
路径 A：自有 gRPC（一等公民）
  .proto → buf/codegen（如 protoc-gen-go-mcp）
       → 进程内 Register/Forward
       → ProtoToolPack(builtin)

路径 B：用户上传 proto
  .proto / FileDescriptorSet 上传
       → 解析 method → JSON Schema
       → 配置 gRPC endpoint + 凭据（存 Gateway）
       → ProtoToolPack(user)

两者都进入 Catalog → 可选入不可变 ToolBundle → Gateway 暴露
```

| 形态 | 用途 |
|---|---|
| **Compile-time（Redpanda 风格）** | 自有服务：生成 Go handler，与 Server/Gateway 同进程或旁路服务 |
| **Descriptor 导入 + 可选 annotation** | 用户上传：运行时根据 descriptor 建 schema 与 forward |
| **（可选）Reflection** | 作为用户路径的增强，非唯一依赖 |

统一执行面：

```text
Agent ──MCP──► DARS Gateway ──┬──► Upstream MCP (stdio/http)
                               ├──► 自有 gRPC（codegen handler）
                               └──► 用户 proto 对应 gRPC Client
```

---

## 端到端产品故事（已定）

```text
配置时（恢复的 MCP 配置页）
  选 MCP sources / 选 Proto packs（自有 + 上传）/ 勾选 tools
       → 物化或复用不可变 ToolBundle(id)
       → Agent 绑定 tool_bundle_id

运行时
  Claim 改写 mcp_config → 只指向 DARS Gateway(bundle_id, dat_)
  Agent 发现的 tools = Bundle 投影
  tools/call 经 Gateway：鉴权 dat_、审计、用 Gateway 侧 secret 路由
```

---

## 实现分期建议（讨论用，非任务拆解）

1. **骨架**：Tool Catalog / ToolBundle 数据模型 + 不可变物化 API  
2. **Facade**：MCP Gateway（`tools/list|call`）+ `dat_` 鉴权 + secret 托管
3. **Claim 改写**：下发 Gateway-only `mcp_config`  
4. **Proto 双路径**：自有 codegen 注册 + 用户 descriptor 导入  
5. **UI**：恢复 Agent MCP 配置页，串起 1–4 做验证  

定稿后可收成 OpenSpec proposal。
