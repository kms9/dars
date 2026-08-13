# lightweight-runtime-release Specification

## Purpose
定义 Lightweight Server 与 Daemon 的允许后台能力、运行恢复、可复现轻量化指标和发布证据门槛，确保“精简”由真实产物与进程行为证明。
## Requirements
### Requirement: 后台 Worker 使用严格白名单
目标 Server SHALL 只运行 runtime/task sweeper（stale runtime/task recovery、queued expiry、deferred chat cancellation finalization、过期凭据回收）、batched heartbeat scheduler、realtime in-memory hub，以及配置后才启用的 Redis realtime relay 和 metrics HTTP listener。Deferred Task promotion SHALL 位于 Claim/Task Core，而非通用 Scheduler。

过期凭据回收 SHALL 由同一 sweeper 承担，删除已过期或已使用的 `verification_code` 以及已过期的 Daemon Token 与 Task Token 行；它 MUST NOT 删除 PAT（PAT 过期后仍需返回 401 并保留用户可见记录），也 MUST NOT 成为独立 Worker。

目标进程 MUST NOT 构造或启动 DB Stats Logger、Task Usage Rollup Scheduler、Notification/Subscriber Worker、Autopilot、Webhook Worker、PR Refresh、Channel Supervisor/Router/Media Reconciler 或其他退出能力 Job；metrics listener 不得执行 business rollup sampler。

#### Scenario: 默认启动 Worker
- **WHEN** Server 使用无 Redis、无独立 metrics listener 的最小配置启动
- **THEN** 启动清单只包含 sweeper、batched heartbeat 与 in-memory realtime hub

#### Scenario: 可选 Worker
- **WHEN** 显式配置 REDIS_URL 或 metrics listener
- **THEN** 只额外启动对应 relay/listener，且不引入 business rollup 或其他 Worker

#### Scenario: Worker allowlist 验收
- **WHEN** 验收检查构造图、已注册 jobs、启动日志和 goroutine 标签
- **THEN** 观察集合与批准白名单精确匹配

### Requirement: Runtime 与 Task 故障必须收敛
Sweeper 和 Task Core SHALL 恢复 stale Runtime/Task、到期 queued Task、prepare lease、`waiting_local_directory`、orphan Task 和 deferred chat cancellation。WS wakeup 不可靠时，HTTP claim/poll SHALL 仍能达到同一权威数据库状态。

#### Scenario: Daemon 异常退出与恢复
- **WHEN** Daemon 在 dispatched、waiting_local_directory 或 running 阶段异常退出
- **THEN** Task 经过冻结的 lease/recovery 规则恢复、有限重试或失败收敛
- **THEN** 不出现永久 running 或重复成功结果

#### Scenario: Deferred Chat Cancel
- **WHEN** Chat cancel finalize 被延迟且 Daemon 未及时确认
- **THEN** sweeper 最终把 Task 与 Draft Restore 收敛到一致状态

#### Scenario: 过期凭据回收
- **WHEN** 验证码过期或用尽、Daemon Token 与 Task Token 过期后 sweeper 运行
- **THEN** 这些行被删除且不影响有效凭据与 PAT 记录
- **THEN** 回收不引入白名单外的独立 Worker

### Requirement: 启动 readiness 达到冻结时限
在 PostgreSQL 已 Ready 且配置合法的同一测量环境中，Server MUST 在 3 秒内达到 `/readyz` Ready；目标 Runtime 已安装时，独立 Daemon MUST 在 5 秒内完成进程启动、注册与 Heartbeat Ready。readiness MUST 包含数据库身份、schema、邮件和 Agent Secret 等生产前置条件，而不只是进程存活。

#### Scenario: Server Ready 时间线
- **WHEN** 从独立进程启动 Server，并持续检查端口与 `/readyz`
- **THEN** 在 3 秒内观察到 Ready，且此前所有强制 guard 已通过

#### Scenario: Daemon Ready 时间线
- **WHEN** 在目标 Runtime 已安装时启动独立 Daemon
- **THEN** 在 5 秒内观察到进程、注册、Daemon Token 和 Heartbeat 全链路 Ready

### Requirement: 轻量化比较使用可复现 clean baseline
首个业务代码变更前 MUST 从基准提交 `736fbc8a5f1b22d48354a0e55baa00661e9e4326` 的 clean detached worktree 保存 baseline，包括 commit、OS/arch、Go/Node/pnpm 版本、构建命令、配置、样本次数与原始产物 hash。目标版本 MUST 使用同一脚本和参数；缺失可复现 baseline 时不得计算百分比或通过发布门禁。

#### Scenario: Baseline 不完整
- **WHEN** 比较记录缺少 commit、环境、参数、样本或原始 hash 任一项
- **THEN** 发布门禁标记为未验证而不是推断通过

### Requirement: 产物和空闲内存达到量化阈值
使用可比 baseline 时，目标 Server binary 字节数 SHALL 不超过基线的 70%，Web production JS SHALL 不超过基线的 60%，Server 稳态 idle RSS SHALL 不超过基线的 80%。

#### Scenario: 量化产物验收
- **WHEN** 在同一 OS/arch、工具链、配置和采样方法下构建并测量基线与目标
- **THEN** 三项比率分别满足 70%、60% 与 80% 上限
- **THEN** 原始产物、build report 和进程探针数据被留存

### Requirement: Skills 上传与管理进入发布验收
发布验收 SHALL 覆盖 Skills 手动创建、URL 导入、archive 上传、Runtime local-skills 导入、文件编辑与 Agent 绑定的正向路径；并覆盖非法 archive、非 Owner local import、删除仍绑定 Skill 的负向路径。

#### Scenario: Skills 导入闭环
- **WHEN** 在真实独立 Server/Daemon/Web 环境执行 URL 或 archive 导入，以及 Runtime local-skills 导入
- **THEN** workspace Skills 列表出现结果，且可绑定 Agent 后被 Task 执行解析

### Requirement: 核心闭环使用真实独立进程验证
发布验收 MUST 使用真实 PostgreSQL、独立 Server 端口、独立 Daemon 进程、真实目标 Runtime Handler、HTTP/WS 请求和 Web 浏览器完成 P0 闭环。证据 SHALL 同时包含进程、端口、HTTP/WS、数据库状态与下游 Runtime Handler 的实际触发，不得只使用 mock、单元测试或 HTTP 自述。

#### Scenario: 真实 Leader-Member-Leader
- **WHEN** 浏览器用户创建 Squad Run，Leader 委派 Member，Member 写回并重新触发 Leader
- **THEN** 独立 Daemon/Runtime 实际执行每个 Task，Issue 最终进入 `in_review`
- **THEN** 数据库、Task lifecycle、Comments 与 Web UI 证据一致

#### Scenario: 真实并发 Claim
- **WHEN** 两个独立 Daemon 对真实 PostgreSQL 并发 Claim 同一 Task
- **THEN** 仅一个 Daemon 获得 Task，完成/失败重放仍幂等

### Requirement: 删除能力以负向证据验收
发布验收 SHALL 覆盖仍退出能力的页面、导航、深链、API、CLI、内置 Skill、依赖和后台 side effect；代表路径 MUST 返回404，旧模块不得被间接构造或通过动态入口恢复。Desktop专属页面和构建不在本阶段验收范围。

Builder与 Agent/Squad avatar SHALL 从退出矩阵移入正向/安全矩阵；Agent MCP管理、Lark/Slack/其他 Integrations、Human Squad Member、Agent Template、Composio、通用 Attachment、Inbox/Channel、Cloud Runtime、Autopilot等仍属于负向矩阵。

#### Scenario: 负向矩阵
- **WHEN** 自动化遍历冻结退出族的代表页面、路径、命令和事件
- **THEN** 所有仍退出入口均不可用，且日志、Router、Worker和依赖扫描无旧能力执行证据

#### Scenario: 防止相邻模块随恢复能力回流
- **WHEN** 验收使用 Builder、avatar、Agent Instructions/Skills或 Squad Agent roster/Instructions
- **THEN** 流程成功，但 MCP/Integrations/Human roster/Template/Attachment browser/Inbox/Channel/Composio代表入口仍为404且相关 Worker未启动

### Requirement: 发布矩阵必须完整通过
发布前 MUST 通过 `docs/prd/dars_lightweight_runtime_docs/04_e2e_acceptance_matrix.md` 中全部 P0、全部安全类 P1、FLOW-01、FLOW-02、CM-07、RC-01、RC-02、TQ-01、TQ-03、TQ-04、API-01..10、PRC-01..07、BLD-01..05 与 DB-01..10；同时不得出现跨 Workspace 泄露、Leader 自触发无限循环或运行期 Comment 永久丢失。

“全部安全类 P1” SHALL 冻结为以下 15 条，不得在发布时重新解释范围：`SEC-03 SEC-04 SEC-05 SEC-06 SEC-07 SEC-08 SEC-09 SEC-10 SEC-11 AUTH-02 AUTH-03 WS-03 WS-04 WS-06 DR-11`。同族的 `SEC-01 SEC-02 SEC-12 AUTH-01 AUTH-04 WS-01 WS-02 WS-05 DR-10` 为 P0，已由“全部 P0”覆盖。

#### Scenario: 安全类 P1 判定
- **WHEN** 发布流程统计强制 ID 通过情况
- **THEN** 上述 15 条安全类 P1 逐条有结果，未运行或跳过任一条即阻止发布

#### Scenario: 生成发布证据包
- **WHEN** 候选版本完成验收
- **THEN** 每个强制 ID 都关联测试结果与可复查原始证据
- **THEN** 任一强制 ID 未运行、跳过或失败都会阻止发布

### Requirement: 标准检查覆盖 Fresh Database 和目标 Web
`make check` SHALL 从 Fresh Checkout 与独立 Lightweight database 验证 Go、目标 TypeScript 工作集、Web build、contract tests 和浏览器 E2E；真实 PostgreSQL 测试不得因本地服务缺失而被当作通过。

#### Scenario: 数据库测试被跳过
- **WHEN** PostgreSQL 前置条件不可用导致 Claim、Handler 或 baseline 测试 skip
- **THEN** 工程检查可报告局部结果，但发布门禁保持未通过

### Requirement: 工程验证与发布结论分离
局部单元测试、静态扫描、OpenSpec 校验或一次性 runtime probe SHALL 只证明其对应检查，不得单独声明生产或发布完成。最终结论 MUST 标注测量时间、环境、版本与未运行门禁。

#### Scenario: 仅完成局部验证
- **WHEN** 实施阶段只运行了 lint、unit test 或 Router snapshot
- **THEN** 状态报告列出已验证项与剩余真实进程/数据库/浏览器门禁，而不标记 change 已发布

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

### Requirement: Gateway 发布证明 Daemon 与 Provider 适配未修改
发布验收 SHALL 证明本变更未修改 `server/internal/daemon/**`、`server/internal/daemon/execenv/**`、`server/pkg/agent/**`、`server/cmd/dars/**` 或 Daemon wire protocol，并证明 `dars` Daemon/CLI 构建依赖图不包含 Server Gateway、MCP SDK、gRPC/OpenAPI/Proto compiler 新依赖。

兼容矩阵 SHALL 使用发布基线的未修改 Daemon 与每个声明支持 Remote HTTP MCP 的目标 Provider Runtime 完成真实 Bundle Claim、MCP discovery 和至少一次受控 tool call，并证明完整 Bundle path 与 Authorization header 被保留。不支持该 MCP config/transport 的 Provider SHALL 在 Server Bundle publication/dispatch 前 fail closed，MUST NOT 通过临时 Daemon patch、custom args 绕过或“配置已下发”自述标记为支持。

#### Scenario: 未修改 Daemon 完成调用
- **WHEN** 独立 Server/PostgreSQL 与发布基线未修改 Daemon 启动目标 Provider Task
- **THEN** Claim 仅通过既有 `agent.mcp_config` 投影 Gateway-only Server document，Runtime 实际完成 `/bundles/{bundleId}/mcp` discovery 和 tool call
- **THEN** Daemon diff、protocol DTO 与依赖图保持冻结

#### Scenario: Provider 不支持 Remote HTTP MCP
- **WHEN** Agent 绑定到现有 Provider adapter 明确无法消费 Remote HTTP MCP 配置的 Runtime
- **THEN** Server 阻止 Gateway Bundle publication/dispatch 并返回可诊断、无 Secret 的 unsupported result
- **THEN** 任务不得在缺少 tool 能力时静默运行，也不得修改 Daemon 扩大支持

### Requirement: 每类 Invoker 使用受控本地 fixture 闭环验收
发布矩阵 SHALL 分别使用测试环境控制的本地 HTTP service、unary gRPC service、uploaded Proto artifact 与 Remote Streamable HTTP MCP fixture，证明 Tool Source validation、revision publish、Bundle publication、Task pin、Claim projection、`tools/list`、`tools/call`、受控 side effect/result、audit 和禁用撤权闭环。仅 parser unit test、mock invoker、静态 Tool Definition、HTTP start event 或 MCP 自述 MUST NOT 计为闭环完成。

该矩阵 SHALL 只证明 DARS Web Server 后端的物化、协议、鉴权、路由和结果映射，不以任何非受控外部依赖服务的当前可用性作为 required gate，也不得把本地 fixture 通过表述为外部服务 uptime/behavior 证明。

#### Scenario: 三类 Invoker 正向闭环
- **WHEN** Active Agent Task 依次调用 ready OpenAPI、gRPC 与 Remote MCP tools
- **THEN** 对应本地 fixture 观察到唯一受控调用，MCP result、数据库 Source revision/Bundle/Task pin 与 redacted audit 一致

#### Scenario: 运行中撤销 Bundle 或 Source
- **WHEN** Admin 在 active Task 已完成一次调用后撤销 Bundle、Agent Gateway access、Source 或 tool
- **THEN** 后续 discovery/call 立即不再获授权，且本地 fixture 不收到第二次调用

### Requirement: Bundle 生命周期进入 unchanged-Daemon E3
E3 matrix SHALL 证明一次语义配置变更生成新 Bundle ID，配置变化前创建的 Task 在首次 Claim、Token 重签发和重新 Claim 后仍使用原 Bundle，而变化后创建的 Task 使用新 Bundle。矩阵 SHALL 同时证明 Bundle Task 的 Server-managed Claim document 只含 Gateway entry，Daemon 既有 runtime-local MCP merge 行为未被修改。

#### Scenario: 配置切换不改变运行中 Task
- **WHEN** T1 固定 B1 后 Agent 发布 B2，随后 T1 重新 Claim 且新建 T2
- **THEN** T1 的 Gateway URL、`tools/list` 与 `tools/call` 仍使用 B1，T2 使用 B2
- **THEN** 两个 Task 的 Token 不能交叉访问另一个 Bundle

### Requirement: Gateway 安全负向矩阵进入 required release gate
Required release gate SHALL 覆盖跨 Workspace/Agent/Task/Bundle 调用、Bundle ID 枚举与 Token/Task pin mismatch、终态/过期 Token、Control Plane 越权、不可变写入尝试、malformed MCP、schema invalid arguments、tool/endpoint override、SSRF、DNS rebinding、redirect、private/metadata IP、TLS identity、oversized artifact/request/response、timeout、cancellation、concurrency、Secret redaction、non-idempotent retry、retained artifact 与 Workspace delete cleanup。

#### Scenario: 安全矩阵执行
- **WHEN** 自动化对真实 Server 数据面与控制面执行全部冻结负向用例
- **THEN** 每个用例 fail closed、无跨租户泄露、无未授权上游连接/副作用、无 Secret 输出且无部分持久化

### Requirement: Gateway E3 证据与普通 Server 检查同时通过
变更发布前 SHALL 通过 Server unit/integration tests、MCP conformance/transport tests、Go build/vet/test、Fresh DB migrations、`make check` 目标流水线，以及独立 Server/Daemon/Runtime E3 matrix。E2/build 成功与 E3/runtime 证据 MUST 分开记录；任何一侧缺失都不得宣称本变更 production-ready。

#### Scenario: 汇总发布证据
- **WHEN** 候选版本准备发布
- **THEN** 报告分别列出静态/单测、Fresh DB/Server integration、未修改 Daemon/真实 Runtime 和受控本地 fixture 证据
- **THEN** 缺失 Provider、网络或安全证据的能力明确标记未完成，并明确外部依赖可用性不在本阶段证明范围
