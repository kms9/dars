# Multica 端到端验收用例矩阵

> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-06`（Agent/Squad §8 工程证据更新）  
> 目标数据库：`multica_lightweight`  
> 文档状态：`目标验收矩阵 / P0-P2、构建、数据库与量化门槛已冻结`

## 0. 当前证据边界

- 当前代码中已存在 Squad Leader 触发、Member Comment 唤醒 Leader、自触发抑制、Comment Reconciliation、Workspace Claim Guard、并发 Claim 和 Complete/Fail 幂等等测试；
- `restore-agent-squad-management-flows` §8 已新增 Agent/Squad 授权、Archive/Builder finalize、Env/avatar redaction、MCP/Lark/Slack/Human Member 负向扫描，以及 126-route / 27-table / worker allowlist 的 contract tests（见 `server/internal/lightweightapi/*_contract_test.go`、`packages/core/lightweight/*contract*.test.ts`、`apps/web/*contract*.test.ts`）；
- **已验证（2026-08-06，本机）**：`pnpm typecheck`、`pnpm test`（target Web/core/ui/views）、`go test ./cmd/server`（route=126）、`go test ./lightweight` + `./internal/lightweightmigrations`（tables=27）、`openspec validate restore-agent-squad-management-flows --strict`；
- **未运行 / 环境门禁**：`LIGHTWEIGHT_DATABASE_URL` 未配置，§8.1–8.2 的 PostgreSQL live contract tests（`TestAgentSquad*Contract*`）全部 `SKIP`；`make check`（隔离 Fresh DB + Playwright + 全量服务）未执行；真实 Daemon/Runtime 浏览器矩阵（§9.1–9.3）未执行；
- **`make test` 现状**：`server/internal/service` 的 `TestLightweightBuiltinSkillProtocolContract` 在本仓库当前状态下失败（与本次 Agent/Squad 变更无直接关系）；修复前不得把 `make test` 记为通过；
- Desktop 专属修改与验收不在当前阶段范围，也不要求兼容旧 Desktop；
- 因此下表是发布目标；§8 contract tests 为工程证据，§9 真实流程矩阵仍为未完成的发布门禁。

## 1. 验收级别

| 级别 | 含义 |
|---|---|
| P0 | 不通过则目标系统不可运行 |
| P1 | 核心可靠性或安全能力 |
| P2 | 可用性和管理能力 |

## 2. Daemon 与 Runtime

| ID | 级别 | 场景 | 前置条件 | 操作 | 预期 |
|---|---|---|---|---|---|
| DR-01 | P0 | 启动 Local Daemon | 用户已登录 | 使用当前保留的 Daemon/CLI 启动方式 | Daemon 独立进程启动并 Health Ready |
| DR-02 | P0 | Runtime 注册 | 本地安装 Codex/Claude | Daemon 启动 | Server 出现对应 Runtime |
| DR-03 | P1 | Runtime 离线 | Runtime 已注册 | 停止 Daemon | Web 显示 Offline |
| DR-04 | P1 | Runtime 恢复 | Runtime Offline | 重启 Daemon | 原 Runtime 恢复或正确重新注册 |
| DR-05 | P1 | 多 Runtime | 一台设备多个 Provider | 启动 Daemon | Web 按设备展示多个 Runtime |
| DR-06 | P1 | Workspace 隔离 | 两个 Workspace | Daemon 使用 WS-A Token | 不得访问 WS-B Runtime/Task |
| DR-07 | P1 | Prepare Lease | Task dispatched，目录准备较慢 | Daemon 延长 lease | 不被其他 Runtime 重领 |
| DR-08 | P1 | Wait Local Directory | 同目录已有运行 | Task 进入等待 | 状态可见且之后能安全启动 |
| DR-09 | P1 | Deferred Promotion | 存在到期 deferred Task | Daemon Claim | 原子晋升并只 Claim 一次 |
| DR-10 | P0 | Daemon 配对 | Human JWT/PAT 调用 Register | 保存返回的 Daemon Token 后继续 Heartbeat/Claim | Token 仅返回一次且只访问绑定 Workspace/daemon |
| DR-11 | P1 | Daemon 撤销/过期 | Deregister 或 Token 到期 | 再次 Heartbeat/Claim | 401；只有 Human JWT/PAT 重新配对可恢复 |

## 3. Auth、Workspace、Runtime Profile 与 Skills

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| WS-01 | P0 | Workspace 选择 | 登录并选择 Workspace | 后续请求携带正确 Workspace Context |
| WS-02 | P0 | Membership Guard | 非成员访问 Workspace API | 404/拒绝且不泄露存在性 |
| WS-03 | P1 | Workspace 删除 | 删除测试 Workspace | 核心数据显式清理，无孤儿数据或跨租户影响 |
| WS-04 | P1 | Member Role | 普通成员修改 Runtime Profile | 拒绝；Admin/Owner 成功 |
| WS-05 | P0 | 首次启动 | 新邮箱完成验证码登录，再创建 Workspace | Verify 只创建 User；Workspace 与 Owner Member 在第二个事务内原子创建 |
| WS-06 | P1 | Invitation 退出 | 请求 Invitation/Member Mutation 路由 | 404 |
| AUTH-01 | P0 | 邮箱验证码 | 请求并在 10 分钟内验证 | 只存 hash，成功建立 Human Session，验证码立即失效 |
| AUTH-02 | P1 | 验证码防重放 | 错误尝试 5 次、过期或成功后重放 | 统一拒绝，不泄露邮箱是否存在 |
| AUTH-03 | P1 | PAT 生命周期 | Create/Renew/Revoke 后调用 CLI API | 明文只返回一次；过期/撤销 Token 401；日志仅有 prefix |
| AUTH-04 | P0 | 生产邮件配置 | 生产模式缺少 Email Provider 启动 | `/readyz` 不 Ready；不得回显或记录明文验证码 |
| RP-01 | P0 | Runtime Profile CRUD | 创建并绑定自定义 Profile | Daemon 能读取并注册 Runtime |
| SK-01 | P0 | Skill CRUD | 创建带文件 Skill | 可读取并绑定 Agent |
| SK-02 | P1 | Skill Bundle Claim | Agent 执行 | Daemon 得到正确且有权限的 Skill Bundle |
| SK-03 | P1 | Invocation Allowlist | `public_to` Agent | 仅允许目标 Member/Workspace 调用 |

## 4. Agent

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| AG-01 | P0 | 创建 Agent | 选择有效 Runtime 创建 | 创建成功且绑定 Runtime |
| AG-02 | P0 | Runtime 必填 | 不选 Runtime 提交 | 400/明确错误 |
| AG-03 | P1 | 跨 Workspace Runtime | 传入其他 Workspace Runtime | 拒绝 |
| AG-04 | P1 | Runtime Offline | 绑定 Offline Runtime 执行 | 明确阻塞或保留队列，不静默丢失 |
| AG-05 | P1 | Archive Agent | 归档后执行 | 拒绝 |
| AG-06 | P1 | 私有 Agent | 非授权用户调用 | 403 |
| AG-07 | P2 | Skills 绑定 | 创建并绑定 Workspace Skills | Claim Payload 正确包含（MCP 配置 UI/API 不在 Lightweight 范围） |

## 5. Direct Chat

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| CH-01 | P0 | 新建 Chat | 选择 Agent 发送消息 | 创建 Session、Message、Task |
| CH-02 | P0 | Claim Chat Task | Daemon Claim | `issue_id` 为空，`chat_session_id` 有效 |
| CH-03 | P0 | Streaming | Runtime 输出流 | UI 实时展示 |
| CH-04 | P0 | Complete | Runtime 完成 | 创建 Assistant Message |
| CH-05 | P1 | Fail | Runtime 返回错误 | 创建可见 Failure Message |
| CH-06 | P1 | Cancel Queued | 用户取消 | Task Cancelled，输入可恢复 |
| CH-07 | P1 | Cancel Running | 用户取消 | Daemon 收到取消并确认 |
| CH-08 | P1 | Resume | 连续发送两轮 | 使用安全 Session/WorkDir |
| CH-09 | P1 | Runtime Missing | Agent 无 Runtime | 409，禁止执行 |
| CH-10 | P2 | Archive Session | 归档后发送 | 拒绝，历史保留 |
| CH-11 | P1 | Attachment 退出 | 发送 Attachment 字段或访问 Attachment 路由 | 400/404，纯文本消息不受影响 |

## 6. Squad 配置

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| SQ-01 | P0 | 创建 Squad | 选择 Leader 创建 | Squad 创建成功 |
| SQ-02 | P0 | Leader 自动入组 | 创建 Squad | Leader Member Role=`leader` |
| SQ-03 | P0 | 添加 Agent Member | 添加成员和 Role | 成功 |
| SQ-04 | P1 | 跨 Workspace Leader | 选择外部 Agent | 拒绝 |
| SQ-05 | P1 | 无调用权限 Leader | 普通成员选择不可调用私有 Agent | 拒绝 |
| SQ-06 | P1 | Archived Squad | 向 Archived Squad 提交任务 | 拒绝 |
| SQ-07 | P2 | Member Status | Member 有 Running Task | Squad 页显示 Working |
| SQ-08 | P1 | Archive 无隐式转移 | 归档只有历史终态 Issue/Task 的 Squad | 不调用或引用 Leader/Autopilot Assignee Transfer |
| SQ-09 | P1 | Human Member 退出 | 添加非 Agent Member | 400，未写入成员记录 |
| SQ-10 | P1 | Archive Active Squad | Squad 存在 Active Task | 409，Squad 和 Issue Assignee 不变 |
| SQ-11 | P1 | Archive Idle Squad | Squad 无 Active Task | Archive 成功，历史 Issue 保留原 Squad Assignee |

## 7. Lightweight Issue

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| IS-01 | P0 | 创建 Squad Issue | Title/Description/Squad | 创建成功 |
| IS-02 | P0 | 自动触发 Leader | Issue status=`todo` | 创建 Leader Task |
| IS-03 | P1 | Backlog | 创建为 backlog | 不立即触发 |
| IS-04 | P1 | Backlog→Todo | 更新状态 | 触发 Leader |
| IS-05 | P1 | 非法 Assignee | Squad 不存在/Archived | 拒绝 |
| IS-06 | P1 | 状态闭环 | Leader 设置 in_review | 状态正确 |
| IS-07 | P1 | Task Complete 非 Issue Done | Leader 首轮仅委派 | Issue 保持 in_progress |
| IS-08 | P0 | Run List | 存在多条历史 Issue | 列表可分页发现并进入详情 |
| IS-09 | P1 | Blocked 状态 | 创建、读取并更新 blocked Issue | 后端与 UI 原生展示，且不自动改为 done/cancelled |
| IS-10 | P1 | Cursor 稳定性 | 相同 `updated_at` 的 Issue 跨两页 | 无重复、无遗漏，按 `updated_at DESC,id DESC` |
| IS-11 | P1 | Run Filter | 组合 status/assignee filter | 只返回 Workspace 内匹配数据 |

## 8. Comment 与 Mention

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| CM-01 | P0 | 添加普通 Comment | 用户添加补充 | Flat Comments 出现 |
| CM-02 | P0 | Agent Mention | Leader 发布 `mention://agent` | 创建目标 Member Task |
| CM-03 | P0 | Squad Mention | 发布 `mention://squad` | 只触发目标 Squad Leader |
| CM-04 | P1 | Plain @name | 只写普通文本 | 不触发 |
| CM-05 | P1 | 重复 Mention | 重复提交同一触发 | 不创建重复 Active Task |
| CM-06 | P1 | 跨 Workspace Mention | Mention 外部 Agent | 拒绝 |
| CM-07 | P1 | Leader Self-trigger | Leader 发布委派 Comment | 不重新触发自己 |
| CM-08 | P1 | source_task_id | Member Task 写结果 | Comment 关联正确 Task |
| CM-09 | P1 | Workspace Integrity | Issue/Workspace 不匹配 | Comment 创建失败 |
| CM-10 | P1 | Comment 不可变 | 调用 Update/Delete/Resolve/Reaction | 404，原 Comment 不变 |
| CM-11 | P1 | Flat Comments | Comments 与 Task Runs 同时存在 | 两个独立分区稳定分页，无 `/timeline` 依赖 |

## 9. Leader—Member—Leader 核心闭环

| ID | 级别 | 场景 | 步骤 | 预期 |
|---|---|---|---|---|
| FLOW-01 | P0 | 单成员委派 | 用户→Leader→Member→Leader | Leader 能最终进入 in_review |
| FLOW-02 | P0 | 两阶段委派 | Leader→Backend→Leader→Test→Leader | 顺序闭环完整 |
| FLOW-03 | P1 | 多成员并行 | Leader 同时 Mention A/B | 两个独立 Task，无重复 |
| FLOW-04 | P1 | Member Failure | Member Task Failed | 用户和 Leader可见失败 |
| FLOW-05 | P1 | Leader Runtime Offline | 创建 Issue | Issue/Task 不丢，状态明确 |
| FLOW-06 | P1 | 无合适成员 | Leader 无法委派 | 写明能力缺口，不静默执行 |
| FLOW-07 | P1 | Human 补充 | Member 执行中用户补充 | 补充不丢失，后续可重入 |

## 10. Comment Reconciliation

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| RC-01 | P1 | Running 中用户 Comment | Agent 已 Running 时新增 | Task 完成后创建必要 Follow-up |
| RC-02 | P1 | Running 中显式 Agent Mention | 目标已有 Running Task | 不丢 Mention，完成后补偿 |
| RC-03 | P1 | 自己的结果 Comment | Agent 完成时写 Comment | 不错误触发自己 |
| RC-04 | P1 | 多条新 Comment | Running 中连续新增 | 按顺序或合并处理，无永久遗漏 |

## 11. Task Queue

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| TQ-01 | P0 | 单次 Claim | 两个 Daemon 并发 Claim | 只有一个成功 |
| TQ-02 | P1 | WS Claim 结果不确定 | WS 发送后断线 | 不立即 HTTP 双重 Claim |
| TQ-03 | P1 | Complete 幂等 | 重复 Complete | 不重复写结果 |
| TQ-04 | P1 | Fail 幂等 | 重复 Fail | 不重复重试/Comment |
| TQ-05 | P1 | Retry | 可重试 Infra Error | 创建有限次重试 |
| TQ-06 | P1 | Cancel | Running Task Cancel | Runtime 停止并回报 |
| TQ-07 | P1 | Session Pin | Runtime 创建 Session | Server/Chat/Issue 指针更新 |
| TQ-08 | P1 | Orphan Recovery | Daemon 异常退出 | Task 可恢复或失败收敛 |
| TQ-09 | P1 | Waiting 状态恢复 | waiting_local_directory 时 Daemon 重启 | Task 不丢失、不重复执行 |
| TQ-10 | P1 | Deferred 任务取消 | deferred Task 被取消 | 到期后不会重新晋升 |
| TQ-11 | P1 | Comment Delivery Fence | Complete 与新 Comment 并发 | delivered/coalesced 状态一致且无永久遗漏 |

## 12. 权限与安全

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| SEC-01 | P0 | Daemon Token 越权 | 访问外部 Workspace | 拒绝 |
| SEC-02 | P0 | Task Token 对象边界 | 访问自身与无关 Issue/Task/Chat | 自身白名单动作成功，无关对象返回 404/403 |
| SEC-03 | P1 | Agent 权限绕过 | Leader 通过 Squad 调用私有 Agent | 拒绝 |
| SEC-04 | P1 | Custom Env 普通 GET | 非 Agent Owner 且非 Workspace Owner/Admin 读取 | 不返回明文 |
| SEC-05 | P1 | MCP Secret 跨用户读取 | 非授权用户读取 | 脱敏或拒绝 |
| SEC-06 | P1 | Comment Actor 伪造 | 客户端提交 author/source 字段 | Server 忽略并使用可信 Actor Context |
| SEC-07 | P1 | Agent Env 审计 | Reveal/Update Env | Secret 脱敏且 activity_log 有审计记录 |
| SEC-08 | P1 | Task Token 字段边界 | 更新自身 Issue 的 title/description 或夹带非白名单字段 | 403，Issue 不发生部分更新 |
| SEC-09 | P1 | Resource Owner | Member 修改他人 Agent/Skill/Squad | 403；资源 Owner 或 Admin/Owner 成功 |
| SEC-10 | P1 | Workspace Delete Role | Admin 与 Owner 分别删除 Workspace | Admin 403，Owner 按生命周期规则成功 |
| SEC-11 | P1 | Public Config | 未认证读取 `/api/config` | 只含目标非 Secret allowlist，无 Cloud/Billing/Integration/Desktop 配置 |
| SEC-12 | P0 | Agent Secret At Rest | 写入 Env/MCP Secret 后查询数据库、日志与 Realtime | 仅见版本化密文/redacted metadata；授权 Reveal/Daemon Claim 可正确解密 |

## 13. API 与进程收缩验收

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| API-01 | P0 | 目标 Route Manifest | Dump Router 并与第 18 节比对 | 126 条 method+path 精确相等、无重复，每个保留页面动作都有 canonical API |
| API-02 | P0 | Removed Route | 访问每个退出路由族代表路径 | 统一返回 404，不进入旧 Handler |
| API-03 | P1 | Breaking Contract | 扫描 Server/Web/CLI/Daemon 路径常量 | 使用同一最终 manifest，无 v1/v2 alias |
| API-04 | P1 | Token Matrix | 对每个保留 Handler 运行凭据正反例 | User/Daemon/Task Token 只能访问各自白名单 |
| API-05 | P1 | Error Envelope | 触发 400/403/404/409/503 | 返回稳定 `error.code/message/details` |
| API-06 | P1 | Cursor Contract | 伪造或过期 cursor | 返回 400，不退化成不稳定 offset |
| API-07 | P1 | Daemon Protocol | 非 `lightweight-runtime-v1` 注册 | 拒绝且不创建 Runtime |
| API-08 | P1 | Create 幂等 | 同 key 重放 Issue/Comment/Chat Message；再用不同 Body 重放 | 同 Body 返回同一资源；不同 Body 返回 409 |
| API-09 | P1 | Strict Body | 向目标 DTO 夹带旧字段 | 400 且不发生部分写入 |
| API-10 | P1 | Resource Lifecycle | 对 Workspace/Runtime/Agent/Skill/Chat/Squad/Issue 覆盖 Active 与 Idle 删除/归档 | 精确符合第 18.11 节的 403/409/事务清理规则 |
| PRC-01 | P0 | Server 启动 | 检查启动日志/构造图 | 不构造或启动 Autopilot、Webhook、PR、Channel Worker |
| PRC-02 | P1 | Scheduler | Dump 已注册 Jobs | 不注册删除能力对应 Job |
| PRC-03 | P1 | Realtime | 触发每类目标事件 | React Query 正确更新，且不广播退出事件 |
| PRC-04 | P1 | Web Realtime Envelope | 接收目标 Web 事件 | 含 event_id/workspace_id/occurred_at/actor/payload |
| PRC-05 | P1 | Realtime 重连 | WS 断开后恢复 | invalidate/refetch，最终与数据库一致 |
| PRC-06 | P1 | Worker Allowlist | 检查启动日志和 goroutine 标签 | 只存在批准后台能力 |
| PRC-07 | P1 | Daemon Control WS | 重复/丢失 Wakeup、RPC timeout | request_id 正确关联；无重复 Claim；安全回退 HTTP |

## 14. 构建图验收

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| BLD-01 | P0 | Root Build | `pnpm build` | 只构建 Web 和目标共享包，不执行 Desktop/Mobile/Docs |
| BLD-02 | P0 | Root Typecheck/Test/Lint | 执行根命令 | 不执行 Desktop/Mobile/Docs |
| BLD-03 | P0 | `make check` | Fresh Checkout + Lightweight Env | Lightweight DB、Go、目标 TS、Web E2E 全部通过 |
| BLD-04 | P1 | CI Frontend | 修改 shared Core/View | 只触发目标工作集，Desktop 不是 required check |
| BLD-05 | P1 | Repository Guidance | 检查 CLAUDE/AGENTS/CI 注释 | 不再声明目标必须 Web/Desktop 双平台 |

## 15. 删除能力验收

以下页面、路由和导航不得出现在目标版本：

- Issue Board；
- Issue Table；
- Issue Properties；
- Labels；
- Subscribers；
- Reactions；
- Due/Start Date；
- Calendar；
- Child Issue UI；
- Autopilot；
- Slack；
- Feishu；
- Mobile；
- Billing；
- Inbox；
- External Webhook 管理。
- Cloud Runtime；
- Attachment；
- Human Squad Member；
- Workspace Invitation；
- Agent Template/Builder（旧 Desktop 模板流；Lightweight 使用 `/agents/new/ai` AI Builder，非本表 MCP/Integrations）；
- Runtime Self-update。

同时检查：

- Web route manifest；
- Sidebar、全局搜索、快捷入口和 Modal；
- API Client、CLI 和内置 Skills；
- 文档、i18n、assets 和 package dependencies；
- 深链接访问不会重新加载已删除页面。

`apps/desktop/**` 的页面、导航和构建不属于当前阶段验收项。

## 16. 数据库与升级验收

| ID | 级别 | 场景 | 操作 | 预期 |
|---|---|---|---|---|
| DB-01 | P0 | Fresh Install | 空 `multica_lightweight` 执行目标 baseline | 可完成 P0 主流程 |
| DB-02 | P0 | Database Guard | migrate/Server 连接 PostgreSQL <15、旧库、错误库名或错误 edition marker | 启动失败且不执行 DDL/业务写入 |
| DB-03 | P1 | Workspace Delete | 删除 Workspace | 显式清理所有核心数据，无残留敏感信息 |
| DB-04 | P1 | Backup/Restore | 备份并恢复新库 | edition guard 通过并继续 P0 主流程 |
| DB-05 | P1 | Schema Allowlist | 查询 `pg_catalog` | 恰好 27 张应用表，索引/Trigger 与批准清单一致 |
| DB-06 | P1 | Attribution/Audit | 完成 Agent/Squad Run | originator/accountable/audit 语义完整 |
| DB-07 | P1 | Old DB Isolation | 运行全部验收 | 原完整产品数据库内容和 revision 不发生变化 |
| DB-08 | P1 | Concurrent Index Rule | 审查目标 migrations | 每个 index 独立使用 CONCURRENTLY，无 FK/CASCADE |
| DB-09 | P1 | Reset Guard | 对 `multica` 执行 `make db-reset` | 命令拒绝且旧库不变 |
| DB-10 | P1 | Baseline Constraints | 违反枚举、Task XOR、终态时间、计数或唯一性 | DB/Service 拒绝且事务不产生部分写入 |

## 17. 轻量化量化验收

目标阈值已冻结；实施前必须采集可复现基线，发布时至少比较：

基线必须在首个业务代码变更前，从基准提交的 clean detached worktree 生成并保存：commit、OS/arch、Go/Node/pnpm 版本、构建命令、配置、样本次数和原始产物 hash。目标版本必须使用同一测量脚本/参数；缺少可复现基线时不得计算百分比或通过 Release Gate。

| 指标 | 基线 | 发布目标 | 证据 |
|---|---:|---:|---|
| Server 二进制大小 | 基准提交同参数产物 | ≤ 70% | 构建产物字节数 |
| Web production JS | 基准提交同参数产物 | ≤ 60% | Next build report/压缩产物 |
| Server 空闲 RSS | 基准提交稳态值 | ≤ 80% | 同机同配置进程探针 |
| Server Ready | 本地 PostgreSQL 已 Ready | ≤ 3 秒 | 独立进程/端口/HTTP 时间线 |
| Daemon Ready | 目标 Runtime 已安装 | ≤ 5 秒 | 独立进程/注册/Heartbeat 时间线 |
| 后台 Worker | 当前启动清单 | 只允许批准清单 | 启动日志/进程探针 |
| API 路由 | 当前 Router dump | 精确等于目标 manifest | Router dump/契约测试 |
| 应用数据表 | 当前完整 Schema | 27 | `openspec/contracts/tables.txt` + `go test ./lightweight -run ModelsMatch` |

## 18. 发布门槛

发布前必须全部通过：

- 所有 P0；
- 所有安全类 P1；
- FLOW-01、FLOW-02；
- CM-07；
- RC-01、RC-02；
- TQ-01、TQ-03、TQ-04；
- 无跨 Workspace 数据泄露；
- 无 Leader 自触发无限循环；
- 无运行期间 Comment 永久丢失。

并且必须满足：

- API-01 至 API-10、PRC-01 至 PRC-07；
- BLD-01 至 BLD-05；
- DB-01 至 DB-10；
- Auth/Workspace/Skill/Runtime Profile 的 P0；
- Web 构建、类型检查和目标浏览器 E2E；
- 真实 PostgreSQL 下的 Claim 并发测试；
- 独立 Daemon 进程、端口、HTTP/WS 与下游 Runtime Handler 的真实触发证据；
- `make check`；
- 第 17 节每项轻量化量化指标达到已冻结阈值并留存原始证据。

不要求旧数据库升级或旧 Desktop 兼容；这两项如果未来重新进入范围，必须建立独立需求和验收矩阵。
