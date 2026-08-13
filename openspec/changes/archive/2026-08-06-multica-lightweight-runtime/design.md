## Context

动机见 [proposal.md](./proposal.md#why)，行为契约见本 change 的五个 capability specs。当前基准提交为 `736fbc8a5f1b22d48354a0e55baa00661e9e4326`，需求来源为 `docs/prd/multica_lightweight_runtime_docs/`。

当前实现的主要约束如下：

- `server/cmd/server/router.go` 同时组装目标能力与 Cloud、Storage、Channel、Slack、Lark、Composio、VCS 等外围依赖；`server/internal/handler.Handler` 是大型聚合对象，单纯取消路由仍会构造不需要的服务。
- `server/cmd/server/main.go` 除必要的 realtime、sweeper 与 heartbeat 外，还注册 Autopilot、Webhook、PR Refresh、DB Stats、Task Usage Rollup 和 Channel 类 worker；只让路由 404 不会减少进程开销与副作用。
- `TaskService`、`IssueService`、Comment Trigger、Daemon claim 与 Chat 取消包含已经存在且必须保留的可靠性协议，但也混入 Autopilot、Quick Create、Inbox、Channel、Attachment 和完整 Project/Issue 分支。
- 当前数据库通过 293 个 up migrations 建立完整 schema，query/generated model 仍包含大量退出表；目标必须从空库建立独立 26 表 baseline。
- Daemon Token 的查询、缓存和 middleware 已存在，但当前主配对链仍主要依赖 Human PAT；目标需要接通专用 Token 的签发、撤销和重配闭环。
- `custom_env`、`mcp_config` 当前主要在响应侧脱敏，数据库 JSON 仍需切换为目标密文 envelope；仓库已有 `server/internal/util/secretbox` 可作为加密原语，但缺少 Agent 配置专用 codec 与密钥 readiness。
- 根 `package.json` 当前只排除 Mobile，主 CI 只排除 Docs/Mobile，Desktop 仍位于目标工作集。共享包修改必须继续遵守 CLAUDE.md 的 React Query/Zustand 和 package boundary 约束。

## Goals / Non-Goals

**Goals:**

- 建立一个无 Full/Light 分支的 Lightweight composition root，使 Router、Handler、Service、Worker 和依赖图只包含目标能力。
- 在同步 breaking cutover 中保持 Agent/Chat/Squad/Issue/Comment/Task 可靠性与安全不变量。
- 从新的数据库身份和 baseline 重新生成数据访问层，并用 guard 保证旧库不可被误写。
- 让每个阶段都有可验证入口、停止条件和可恢复的回退点，最后再物理删除旧模块。
- 以机器可比的 manifest、schema、worker 和性能证据决定发布，而不是以代码删除量判断。

**Non-Goals:**

- 不设计旧 API、旧 Desktop 或旧数据库兼容、双写或数据迁移。
- 不在本变更中修改 Desktop 专属源码，也不解决其与新共享包的编译兼容。
- 不引入新的 SquadRun、DAG、工作流引擎或多 Agent Chat 模型。
- 不在 Server 启动时自动安装 schema；安装只由显式 migrate 命令完成。
- 不承诺从 Lightweight 数据库无损回滚到完整产品 schema。

## Decisions

### 1. 使用单一 composition root，不使用 Feature Flag 裁剪

目标版本直接把 `server/cmd/server` 的 composition root 改造成 Lightweight 依赖图。Router 只构造五个 spec 所需的 Handler/Service；外围集成既不注册 route，也不创建 client、subscriber、scheduler 或 goroutine。现有 Feature Flag service 不作为 Full/Light 选择器。

选择原因：功能开关只能隐藏行为，不能证明二进制、RSS、worker 和依赖真正缩小；它还会长期保留两套组合路径。

备选方案：在现有 `NewRouterWithOptions` 周围增加 `LIGHTWEIGHT=true` 分支。拒绝，因为这会违反单一产品和无双模式契约，并扩大测试矩阵。

### 2. 先保存可复现基线，再开始业务代码变更

在 clean detached worktree 的冻结提交上运行统一测量入口，保存 server binary、Web production JS、idle RSS、ready 时间、Router dump、worker dump、schema snapshot，以及环境、命令、样本和 hash。目标版本继续调用同一测量入口，只更换被测 commit/产物。

选择原因：百分比阈值依赖可比证据；在裁剪后补测“基线”无法证明变化。

备选方案：使用历史 CI artifact 或开发机现有 build。拒绝，除非它完整保存了同一参数与原始 hash，否则不可复现。

### 3. 以 manifest 驱动 Router contract，而非从现有 Router 做黑名单减法

实现一个唯一的 Lightweight route registration 表，注册 `lightweight-api-security` 中的 114 条 method+path。Router snapshot、权限矩阵测试、Web API client 和 Daemon/CLI path 常量均与此 manifest 对照；未列入项默认不存在。

路由按认证边界拆为：Public/Auth、Human Workspace API、Task-scoped API、Daemon API 与两个 WS upgrade。middleware 在组级安装，但每个 Handler 仍有正反凭据 contract test。

选择原因：现有 Router 规模大且外围路由持续变化，黑名单容易漏掉 alias、健康子路由或隐式 Mount；白名单能让 API-01 成为精确集合比较。

备选方案：保留全部 Handler，仅逐个删除 route。可作为 Surface 阶段的短暂工作方式，但不能作为最终 composition root。

### 4. 按运行责任拆出 Core，再删除外围分支

从现有 Service 中保留并收口五类责任：

```text
Identity Core       email auth, workspace, membership, PAT/daemon/task token
Agent Core          runtime profile, local runtime, agent, skill, invocation gate
Collaboration Core  squad, lightweight issue, append-only comment, mention/re-entry
Task Core           enqueue, claim, lease, lifecycle, retry, recovery, reconciliation
Chat Core           session, message, streaming projection, cancel/draft restore
```

Handler 负责解析严格 DTO、可信 Actor/Workspace Context 和 HTTP 映射；Core 负责业务不变量与事务；query 层只访问 26 表。Realtime 由事务成功后的 domain event 驱动，不能成为权威状态。

选择原因：直接在现有 `TaskService`/`IssueService` 中删除条件分支，容易连带破坏 lease、comment delivery、attribution 或自触发 guard；先给保留协议建立窄接口和 characterization tests，可分离业务核心与外围调用。

备选方案：一次性重写全部 Service。拒绝，因为可靠性状态组合多，无法在同一阶段区分预期 breaking change 与回归。

### 5. Web 建立 Lightweight route tree，Server state 继续由 React Query 管理

`apps/web` 只挂载保留页面；`packages/views/issues` 与 `packages/core/issues` 被收口为 Run List/Create/Detail、Flat Comments 和 Task Runs。复用组件迁到 `packages/ui`、`packages/views/common` 或相应 Core；不在 Views 引入 Next API，不在 UI 引入 Core。

Server state（Run、Comment、Task、Chat、Agent、Squad、Runtime、Skill）继续由 React Query 拥有；Zustand 只保存筛选、草稿、Modal 等客户端状态。WS event 只更新/失效 React Query，不把 Server entity 复制进 store。

选择原因：遵守现有包边界可避免为 Desktop 保留平台耦合；建立新 Lightweight Run 页面比继续裁剪依赖 Table/Facet/Group 的 `IssuesPage` 更可控。

备选方案：在现有完整 Issue 页面上隐藏控件。拒绝，因为其 bundle、查询、状态模型和深链仍依赖退出能力。

### 6. Human、Daemon 与 Task 凭据采用分离 middleware 链

Email Code 建立 Human Session；PAT 继承 Human 权限并用于 CLI/首次配对。Register 在验证 Human JWT/PAT 后签发一次可见 Daemon Token，后续 `/api/daemon/**` 只接受该 Token。Task Token 由 claim 流程为具体 Task 签发，并从服务端绑定解析 Actor 和上下文。

Token middleware 输出不可被 header/body 覆盖的 typed actor context。每个 route 的允许凭据和拒绝凭据均作为 table-driven contract test；Token 比较统一走 hash，revoke/re-pair 同步失效缓存。

选择原因：当前多种 fallback 会扩大权限面；分离 middleware 能消除 PAT 长期承担 Daemon lifecycle 和客户端伪造 `X-Task-ID` 的风险。

备选方案：继续让 Daemon 使用 PAT，并只依赖 path 内 Workspace 校验。拒绝，因为 PAT 权限过宽，无法满足 daemon-scoped 撤销和隔离。

### 7. Web Realtime 与 Daemon Control 保持两套协议

Web `/ws` 只发送 Workspace entity event envelope；Daemon `/api/daemon/ws` 只发送 control envelope。两者共享 domain event 来源但不共享 wire type。Web 断线后 refetch；Daemon hint 只负责唤醒，HTTP lifecycle 与 PostgreSQL 仍是事实来源。

选择原因：Web 关注缓存一致性，Daemon 关注 claim/RPC 控制；混用 envelope 会让权限、重放和幂等语义不清晰。

备选方案：扩展一个通用 WS envelope。拒绝，因为它会迫使客户端接受无关事件，并诱导 Daemon 把非持久事件当权威状态。

### 8. 新 baseline 与旧 migration 链并行验证后再替换

Baseline 的编写先于 Core Extraction：在隔离路径建立 Lightweight table migrations、每索引一个 concurrent migration、目标 queries 和独立 sqlc 输出，并通过 Fresh Install 与 26 表/index/constraint 审计后，Core 直接基于新 baseline 实现一次数据访问层，避免先对旧 schema 写一遍再整体重映射字段。

运行时切换是独立阶段：Server guard、`migrate` 命令、环境/CI 默认值和真实数据门禁在 baseline 与 Core 就绪后才生效，目标 `migrate` 命令只装 Lightweight baseline。旧 migration/query/generated 文件在 Fresh Install、guard、Workspace Delete、Backup/Restore 与 P0/P1 全部通过后才从目标代码库移除。

Baseline 不使用 FK/CASCADE。跨表关系由 Core 事务和锁保证，行内枚举/XOR/计数约束由 CHECK 与 Service 双层验证。`schema_metadata`、数据库名和支持版本在 migrate/server 两个入口分别 fail closed。

选择原因：先删除 293 个 migrations 会让遗漏字段、索引或删除顺序难以定位；影子构建允许与冻结 26 表契约逐项比对，同时仍保证最终启动链只有新 baseline。

备选方案：在旧 schema 上 Drop Table 或从 migration 294 开始裁剪。拒绝，因为目标不升级旧库，且留下历史表/索引会违反精确 26 表门禁。

### 9. Agent Secret 使用专用版本化 codec

在 Agent 配置边界引入 envelope codec，复用已审计的 authenticated encryption 原语，并显式写入 `v/kid/nonce/ciphertext`。`custom_env` 按 value 加密以支持 key 级更新与 Reveal；`mcp_config` 整体加密以避免结构泄露。读取列表只投影 redacted metadata，只有 Env Reveal 和 Daemon Claim 获取短生命周期明文。

密钥由 `MULTICA_AGENT_SECRET_KEY` 加载并进入 production readiness；codec 支持按 `v/kid` 解密，为未来轮换留出格式，但本变更不实现后台轮换 worker。

选择原因：复用通用 secretbox 原语可减少新密码学风险，同时专用 codec 能固定 JSON envelope 和审计语义。

备选方案：仅磁盘/数据库层加密或继续响应脱敏。拒绝，因为数据库查询和备份仍可见原文。逐字段加密整个 MCP document 也拒绝，因为会暴露结构并增加更新复杂度。

### 10. Build graph 通过正向 filter 固定目标工作集

根 pnpm/Turbo 和主 CI 使用 Web 加目标共享包的正向工作集，而不是逐个排除已知 app；`make check` 组合 Lightweight database、Go、目标 TS、Web contract 与浏览器 E2E。Desktop/Mobile/Docs 可保留独立非 required workflow，但不得被主检查隐式拉入。

选择原因：负向 filter 会在新增 app 时自动扩大目标构建，正向 filter 才能稳定证明发行边界。

备选方案：继续追加 `!desktop !docs !mobile`。拒绝，因为它仍可能把未来非目标 workspace 引入根命令。

### 11. 物理删除由依赖扫描和真实闭环解锁

Surface 与 Core 阶段允许旧文件暂存，但它们不能被 target Router、composition root、build graph 或运行 Worker 引用。只有在 import/query/path/event 扫描、真实 PostgreSQL 并发测试、Daemon/Runtime 与 Web P0 闭环通过后，才删除外围 Handler、Service、query、generated code、migration、CLI command、assets 和 dependencies。

选择原因：分阶段删除把行为回归与编译清理分开，同时最终仍满足单一轻量产品和二进制规模要求。

备选方案：只停止引用但永久保留所有源文件。拒绝，因为无法达到产物规模、依赖和长期维护目标。

### 12. 回滚只切换版本与 DSN，不降级数据库

每个阶段保留可部署的前一发行物和它自己的 DSN。切换到 Lightweight 后，如需回滚完整产品，部署旧二进制并重新指向未修改的旧 `multica` 数据库；新 `multica_lightweight` 数据库冻结为只读诊断/导出源。不得把新库 downgrade、把新数据隐式导入旧库或让两个版本共享 DSN。

选择原因：两套 schema 明确不兼容，数据库 downgrade/双写会制造未冻结的数据映射与破坏风险。

备选方案：提供自动 rollback migration 或双写。拒绝；如未来需要数据迁移，必须另立 change。

### 13. Workspace Context 由 `X-Workspace-ID` 单一建立

Human Workspace-scoped 请求的 Workspace 只从 `X-Workspace-ID` 头的 UUID 解析，middleware 用 Membership Guard 产出 typed context。不接受 slug、不在缺 header 时回退 path/body、不使用 Session 记忆的当前 Workspace。带 `{workspaceId}` 的路由要求 path 与 header 一致，否则 404。Daemon/Task 凭据的 Workspace 只来自 Token 记录并忽略该头。Web 用 `GET /api/workspaces` 自行把 slug 路由解析成 UUID。

选择原因：Email Code 建立的 Session 本身不含 Workspace，必须有一个显式来源才能让“path/body 不得覆盖上下文”成为可测规则。现有 `workspaceIDFromURL` 在 context 为空时回退 chi path 参数，且 slug 与 UUID 双解析，正是需要关闭的歧义面。

备选方案：把全部 Workspace-scoped 路由改为 path 携带 `{workspaceId}`。拒绝，因为它要改动已冻结的 114 条 manifest。让服务端在 Session 内记忆当前 Workspace 也拒绝，因为会破坏多标签页、CLI 与 Daemon 的并发使用。

## Risks / Trade-offs

- [共享 Core/View 裁剪导致 Desktop 无法编译] → Desktop 从 required build graph 排除；不引入兼容 shim，后续另立 Desktop 变更。
- [白名单 Router 已收缩但聚合 Handler 仍构造外围依赖] → Router snapshot 与 composition/worker/import 三类检查同时作为 Surface gate。
- [抽取 Task/Issue Core 时破坏可靠性边缘状态] → 删除分支前先为 claim、lease、deferred、waiting、reconciliation、cancel、attribution 和自触发建立 characterization/并发测试。
- [Daemon Token 签发切换使旧 Daemon 立即失联] → Web/Server/Daemon/CLI 同批发布，协议不匹配明确返回 `protocol_version_unsupported`；不做 silent fallback。
- [新 baseline 漏字段、索引或显式清理顺序] → 用冻结字段/index 清单生成审计，真实 PostgreSQL 跑 Fresh Install、并发 Claim、Workspace Delete 和 Backup/Restore 后才物理删除旧链。
- [没有 FK 会产生孤儿或跨租户引用] → 所有跨表 mutation 放入 Core 事务并加 contract/concurrency test；Workspace Delete 使用显式清单和残留探针。
- [当前依赖 FK 隐含行锁的并发保护在新 baseline 静默失效] → 现有 Runtime 解绑删除依靠 `agent.runtime_id` 外键校验取得的 `FOR KEY SHARE` 阻塞并发改绑；新 baseline 无 FK 后必须改为显式锁定 Runtime 行与相关 Agent 行，并保留事务内重新枚举加集合确认。移植前先审计所有依赖此类隐含锁的写路径。
- [Agent Secret 加密后无法恢复或轮换] → envelope 包含 v/kid，备份恢复把密钥作为独立依赖并执行 decrypt probe；禁止丢失 key 时自动清空或明文降级。
- [一次 breaking cutover 的联调面大] → 使用阶段 gate 和同一 manifest/DTO contract，按 Server → Daemon/CLI → Web 的可运行切片推进，但只发布全部同步后的候选版本。
- [轻量化指标受机器噪声影响] → 固定环境和采样方法、保存多次样本及 raw hash；任一不可比项保持未验证。
- [物理删除过早使回归修复困难] → Physical Delete 只在新 baseline 和真实闭环 gate 后进行；Git/release artifact 与旧库提供代码级回退，而非 DB downgrade。

## Migration Plan

### Phase 0: Contract 与基线冻结

1. 在 clean detached worktree 采集并保存全部规模、进程、Router、Worker 和 schema baseline。
2. 把 114 路由、26 表、Worker 白名单、事件白名单和 142 个验收 ID 变成机器校验输入。
3. 为保留可靠性协议补齐当前行为 characterization tests。

Gate：基线可复现、contract fixtures 无重复且与 specs 一致。回退：仅删除测量产物，不涉及业务状态。

### Phase 1: Surface 收缩

1. 建立唯一 Lightweight Router/composition root，停止注册旧 routes 和构造旧 worker/integration。
2. 建立 Lightweight Web route tree、Run pages 和目标 API client；根构建图排除 Desktop/Mobile/Docs。
3. 同步清理 CLI 与内置 Skills 的旧入口，固定 Web/Daemon event 白名单。

Gate：Router=114、Worker allowlist、removed route 404、目标 Web build/typecheck、无退出页面入口。回退：部署 Phase 0 发行物；数据库仍未切换。

### Phase 2: Core Extraction 与安全切换

1. 在隔离路径完成 Lightweight table/index migrations、目标 queries 与 sqlc 产物，并通过 Fresh Install 与 26 表/index/constraint 审计，使 Core 只针对新 baseline 实现一次数据访问层。
2. 收口 Identity、Agent、Collaboration、Task、Chat Core，删除 Autopilot/Project/Attachment/Channel 等分支。
3. 接通 Email Code、PAT、Daemon Token、Task Token 与 typed actor context；Daemon 切换 `lightweight-runtime-v1` 和唯一 batch claim。
4. 分离 Web realtime 与 Daemon control WS，接通幂等/refetch/HTTP fallback。
5. 实施 Agent Secret codec、redaction、audit 与 production readiness。

Gate：Baseline 影子构建审计通过；API/Token/Realtime contract 全部通过；真实 PostgreSQL 下 claim、lease、reconciliation 与 lifecycle 并发测试通过。回退：部署 Phase 1/0 发行物并使用其 DSN；不复用半成品 token/schema。

### Phase 3: New Baseline 运行时切换

1. 在目标环境从空库安装 Phase 2 产出的 baseline、marker、CHECK 与独立 concurrent index migrations。
2. 让 composition root 与 `migrate` 命令只接受新 schema。
3. 更新环境、Make、Compose、CI、worktree/reset/migrate 命令与 readiness guard。
4. 验证 Workspace Delete、Fresh Install、Backup/Restore 和 Secret decrypt probe。

Gate：`pg_catalog` 恰好 26 表，name/edition/version guard 先于写入，DB-01..10 通过。回退：停止新发行物并保留新库；旧发行物只连接未修改旧库。

### Phase 4: 真实 P0 闭环

1. 启动独立 PostgreSQL、Server、Daemon 和真实 Runtime，验证 Agent、Direct Chat 和 Leader-Member-Leader。
2. 在浏览器验证 Run List/Detail、Flat Comments/Task Runs、WS 重连与退出深链。
3. 验证 Fresh Install 与 Restore 环境重复获得同一结果。

Gate：全部 P0、全部安全 P1 和发布矩阵指定用例有进程/端口/HTTP/WS/DB/Runtime 证据。回退同 Phase 3。

### Phase 5: Physical Delete

1. 删除不可达的外围 Handler、Service、query/generated model、旧 migrations、CLI、Skill、assets、i18n 和 dependencies。
2. 重跑 import/query/path/event/route/worker/schema 扫描和全部 Phase 4 验证。

Gate：无目标外运行依赖，旧库 checksum 未变化，功能与安全门禁不回退。回退：恢复上一候选版本；继续使用相同 Lightweight DB，不执行 downgrade。

### Phase 6: Release

1. 在与 baseline 相同参数下测量 binary、JS、RSS、Server/Daemon ready，并生成证据包。
2. 执行 `make check`、真实 PostgreSQL 并发测试、目标浏览器 E2E、Backup/Restore 和全部强制验收 ID。
3. 仅在所有硬门禁通过后发布；归档旧完整产品数据库为只读资源。

Gate：五个 capability specs 全部满足且证据无 skip。回退：部署前一发行物与其独立数据库；保留 Lightweight DB 等待诊断或显式导出需求。
