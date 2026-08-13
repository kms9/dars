## Purpose

定义 Lightweight Server 与 Daemon 的允许后台能力、运行恢复、可复现轻量化指标和发布证据门槛，确保“精简”由真实产物与进程行为证明。

## ADDED Requirements

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

### Requirement: 路由、Worker 和数据表计数是硬门禁
发布 SHALL 要求 Router dump 精确等于 116 条 manifest、运行 Worker 精确等于批准清单、`pg_catalog` 显示恰好 26 张应用表。页面减少、单元测试通过或自描述 health 响应不得替代这些证据。

#### Scenario: 任一清单漂移
- **WHEN** Router 多/少一条路径、运行中出现额外 Worker，或数据库应用表不是 26 张
- **THEN** 发布门禁失败

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
发布验收 SHALL 覆盖退出能力的页面、导航、深链、API、CLI、内置 Skill、依赖和后台 side effect；代表路径 MUST 返回 404，旧模块不得被间接构造或通过动态入口恢复。Desktop 专属页面和构建不在本阶段验收范围。

#### Scenario: 负向矩阵
- **WHEN** 自动化遍历冻结退出族的代表页面、路径、命令和事件
- **THEN** 所有入口均不可用，且日志、Router、Worker 和依赖扫描无旧能力执行证据

### Requirement: 发布矩阵必须完整通过
发布前 MUST 通过 `docs/prd/multica_lightweight_runtime_docs/04_e2e_acceptance_matrix.md` 中全部 P0、全部安全类 P1、FLOW-01、FLOW-02、CM-07、RC-01、RC-02、TQ-01、TQ-03、TQ-04、API-01..10、PRC-01..07、BLD-01..05 与 DB-01..10；同时不得出现跨 Workspace 泄露、Leader 自触发无限循环或运行期 Comment 永久丢失。

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
