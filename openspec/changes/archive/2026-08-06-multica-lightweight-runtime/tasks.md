## 1. 冻结基线与机器契约（Phase 0）

- [x] 1.1 在基准提交 `736fbc8a5f1b22d48354a0e55baa00661e9e4326` 的 clean detached worktree 记录 OS/arch、Go/Node/pnpm 版本、构建配置和原始 commit 状态
- [x] 1.2 用固定参数构建并保存基准 Server binary、Web production JS 报告和产物 hash
- [x] 1.3 用独立 PostgreSQL 和进程探针采集基准 idle RSS、Server ready 时间、Worker 清单、Router dump 与 schema snapshot
- [x] 1.4 将 114 条 method+path 路由生成无重复的排序 contract fixture，并校验与 `lightweight-api-security` spec 一致
- [x] 1.5 将 26 表、字段/约束/index allowlist、Worker allowlist、Web/Daemon event allowlist 生成机器校验 fixture
- [x] 1.6 将 142 个验收 ID 建立唯一索引和证据状态清单，并把强制发布子集连同 15 条安全类 P1 的显式 ID 列表冻结为机器可校验输入
- [x] 1.7 为 claim、lease、deferred、waiting、complete/fail/cancel、comment reconciliation、attribution 和 Leader self-trigger 补齐现状 characterization tests
- [x] 1.8 保存原 `multica` 数据库身份、revision、schema 与内容 checksum 证据，作为后续只读隔离对照

## 2. 目标构建图与检查入口（Phase 1）

- [x] 2.1 将根 `build/typecheck/test/lint` 改为 Web 与目标共享包的正向 Turbo 工作集
- [x] 2.2 从主 CI required jobs 中排除 Desktop、Mobile、Docs，并保留或建立其非 required 独立 workflow 边界
- [x] 2.3 更新 `make check` 使其编排 Lightweight Fresh Database、Go、目标 TypeScript、Web contract 和浏览器 E2E
- [x] 2.4 更新 Makefile、package scripts、workspace filter 和 CI 的 contract tests，证明根命令不会间接拉入非目标 app
- [x] 2.5 更新 CLAUDE/AGENTS/开发文档中的目标构建说明，删除 Web/Desktop 双平台 required-check 表述
- [x] 2.6 运行并留存 BLD-01..05 证据，确认 Desktop 专属源码未因本阶段被主动改造

## 3. Lightweight Server Composition 与 Router（Phase 1）

- [x] 3.1 建立不含 Full/Light 开关的单一 Lightweight composition root 和最小依赖结构
- [x] 3.2 将 Public/Auth、Human Workspace、Task-scoped、Daemon 与 WS 路由按独立认证边界注册
- [x] 3.3 用唯一 registration manifest 注册全部 114 条目标路由并删除 `/healthz`、realtime metrics JSON 和其他未列入路径
- [x] 3.4 停止在 Router 构造 Storage、Cloud Runtime、Channel、Slack、Lark、Composio、VCS、Attachment 和退出能力服务
- [x] 3.5 停止在 Server main 构造 Autopilot、Webhook、PR Refresh、DB Stats、Task Usage Rollup、Notification/Subscriber 和 Channel 类 worker
- [x] 3.6 仅保留 runtime/task sweeper、batched heartbeat、in-memory realtime hub 及显式启用的 Redis relay/metrics listener
- [x] 3.7 在同一 sweeper 内实现过期验证码与过期 Daemon/Task Token 回收，不新增独立 Worker 且不删除 PAT 记录
- [x] 3.8 从 metrics listener 移除 BusinessSampler 和退出能力 collector，保留进程/HTTP/realtime 基础指标
- [x] 3.9 将 `/api/config` 收口为冻结 allowlist，并把数据库、邮件、Agent Secret 和 schema guard 接入 `/readyz`
- [x] 3.10 实现统一错误 envelope、稳定错误码和 Request ID 关联，过滤 SQL、路径、数据库文本与 Secret
- [x] 3.11 实现严格 JSON DTO、Content-Type、merge/null、unknown/read-only field 和统一 cursor/limit 校验
- [x] 3.12 添加 Router 精确集合、removed route 404、composition graph 和 Worker allowlist contract tests
- [x] 3.13 运行 API-01、API-02、PRC-01、PRC-02、PRC-06 并修复所有目标外构造或注册

## 4. Baseline Schema、索引与数据访问层（Phase 2 前置：隔离路径影子构建）

本组必须在第 5–9 组 Core 实现之前完成，使 Core 只针对新 baseline 写一次数据访问层；新库的运行时切换、环境配置与真实闭环验证留在第 12 组。

- [x] 4.1 在 migrate 入口实现 DDL 前 database name guard、空/非空库分支和 schema marker/version 校验
- [x] 4.2 创建 schema/identity/workspace/token 表 migrations，使用冻结字段、default、CHECK 且无 FK/CASCADE
- [x] 4.3 创建 runtime/agent/skill 表 migrations，使用冻结字段、default、CHECK 且无 FK/CASCADE
- [x] 4.4 创建 squad/issue/comment/task/activity 表 migrations，使用冻结字段、XOR/终态/计数 CHECK 且无 FK/CASCADE
- [x] 4.5 创建 chat session/message/draft restore 表 migrations，使用冻结字段、default、CHECK 且无 FK/CASCADE
- [x] 4.6 为每个 ID、唯一性与访问索引建立独立单语句 `CREATE [UNIQUE] INDEX CONCURRENTLY` migration
- [x] 4.7 用后续 migration `USING INDEX` 绑定需要的唯一约束，确认 table DDL 不隐式创建索引
- [x] 4.8 建立仅访问 26 表的目标 sqlc queries，并覆盖稳定 cursor、claim/lease/deferred、idempotency 和历史 display
- [x] 4.9 从目标 baseline 生成 sqlc models/queries，并把生成产物固定为后续 Core 的唯一数据访问层
- [x] 4.10 在隔离 Lightweight 数据库执行 Fresh Install 与 26 表/index/constraint 静态审计，作为 Core 开发前置门禁
- [x] 4.11 实现 `custom_env` value 与完整 `mcp_config` 的 `{v,kid,nonce,ciphertext}` codec 和数据库持久化

## 5. Identity、Workspace 与 Token 安全（Phase 2）

- [x] 5.1 将 P0 Auth 收口为 Email Code，移除 Google、Contact Sales、Invitation 和旧 Onboarding 流程
- [x] 5.2 实现验证码 hash、10 分钟 expiry、5 次失败上限、single-use 和 send-code 防邮箱枚举
- [x] 5.3 实现 production Cookie 与 CSRF 约束，并确保生产缺少邮件配置时 fail closed
- [x] 5.4 将首次 verify 收口为只创建 User，并让 `POST /api/workspaces` 事务创建 Workspace、Owner Member 与 Issue counter
- [x] 5.5 实现只读 Member List、可信 Workspace Context、Membership Guard 和跨 Workspace 404 语义
- [x] 5.6 将 Workspace Context 收口为 `X-Workspace-ID` UUID 单一来源，移除 slug 解析与 path/body 回退，并对 path 与 header 不一致返回 404
- [x] 5.7 实现 Member/Admin/Owner 与资源 Owner 的冻结权限矩阵和 Workspace Delete Owner-only gate
- [x] 5.8 实现 PAT create/renew/revoke/expiry、一次可见明文、hash/prefix 存储和日志 redaction
- [x] 5.9 接通 Register 的 Human JWT/PAT 配对、`lightweight-runtime-v1` 校验和一次可见 Daemon Token 签发
- [x] 5.10 实现 Daemon Token 的 workspace/daemon/runtime/task scope、过期、Deregister、re-pair 与 Workspace Delete 撤销
- [x] 5.11 将 `/api/daemon/**` 后续请求切换为 Daemon Token 并移除 Human PAT fallback
- [x] 5.12 实现 Task Token 的服务端绑定解析与 Issue/Chat/Comment/Task Message/Squad Evaluation 最小权限
- [x] 5.13 对 Task Token 的 title/description/其他资源/Secret 等越权请求返回 403/404 且无部分写入
- [x] 5.14 统一 Token hash 比较、缓存失效和日志保护，覆盖 revoke、renew、deregister、re-pair 并发场景
- [x] 5.15 为 114 个 Handler 添加允许凭据和拒绝凭据的 table-driven contract matrix
- [x] 5.16 实现 Workspace Delete 显式清理清单、Active Task/Daemon guard 和其他 Workspace/User/PAT 保护
- [x] 5.17 运行 AUTH-01..04、WS-01..06、DR-10..11、SEC-01..11 与 API-04 权限证据

## 6. Daemon 协议、Claim 与实时事件（Phase 2）

- [x] 6.1 将 Daemon/CLI register、heartbeat 和 WS handshake 固定为 `lightweight-runtime-v1`
- [x] 6.2 将批量 Claim 收口到唯一 `/api/daemon/tasks/claim`，删除 `/api/daemon/claim`、单 Runtime Claim 和客户端 legacy fallback
- [x] 6.3 删除 Runtime Self-update result、Autopilot GC 与其他未列入 Daemon manifest 的客户端和服务端路径
- [x] 6.4 让 Claim 原子绑定 Runtime/Task、签发 Task Token 并只返回授权 Context、Skill Bundle 与解密后的执行 Secret
- [x] 6.5 实现 Daemon control envelope、事件白名单、request_id RPC 关联和 HTTP fallback
- [x] 6.6 验证 WS hint 丢失、重复、断线与 timeout 不会导致双重 Claim 或 Task 丢失
- [x] 6.7 实现 Web Workspace event envelope、冻结白名单、event_id 幂等和目标外事件抑制
- [x] 6.8 为 Web WS upgrade 加认证与 Membership Guard，并验证无历史 replay 的 reconnect/refetch 契约
- [x] 6.9 将 `GET /api/daemon/workspaces` 收口为只返回 Token 绑定 Workspace，并对齐 repos 查询的字段与越界语义
- [x] 6.10 实现 Issue/Chat Session/Task 的 GC 状态查询：最小字段、404 即可回收、批量上限与 Token 范围校验
- [x] 6.11 对齐 Server、Daemon/CLI 与 Web 的路径常量、DTO、协议版本和 event types，删除旧 alias
- [x] 6.12 运行 DR-01..11、API-03、API-07、PRC-03..05、PRC-07 的 contract 与故障注入测试

## 7. Agent、Runtime Profile 与 Skill Core（Phase 2）

- [x] 7.1 收口 Runtime Profile CRUD、Local Runtime register/list/update/delete 和 request/result 流程
- [x] 7.2 删除 Cloud Runtime、Runtime Usage、Connected App、Composio allowlist、Agent Template/Builder 和 Runtime Self-update 分支
- [x] 7.3 实现 Agent create/update/archive/restore 的严格 DTO、同 Workspace Runtime 必填与 Active Task guard
- [x] 7.4 保留 Agent model、thinking level、service tier、runtime config、custom args、concurrency、permission 和 disabled runtime skills 契约
- [x] 7.5 收口 Skill/File CRUD 与 Agent-Skill 完整集合绑定，处理绑定引用冲突
- [x] 7.6 实现 private/public_to invocation gate 与 Workspace/Member target 校验，并覆盖 Squad 间接调用
- [x] 7.7 为归档或删除关联对象提供稳定 tombstone display，避免历史 Run 无法渲染
- [x] 7.8 实现 Runtime 删除的绑定 Agent 拒绝、`expected_active_agent_ids` 集合确认与 Profile 派生实例拒绝
- [x] 7.9 用显式 Runtime/Agent 行锁替代原先依赖外键的隐含锁，并覆盖确认期间集合变化的并发测试
- [x] 7.10 实现 Agent Secret key 加载/readiness、Env Reveal/Update audit、Daemon Claim 解密和全局 redaction
- [x] 7.11 运行 RP-01、SK-01..03、AG-01..07 和资源 lifecycle 正反例

## 8. Squad、Run、Comment 与 Task Core（Phase 2）

- [x] 8.1 将 Issue Service 收口为 title/description/status/assignee/acceptance/context 的 Lightweight Run DTO 和状态机
- [x] 8.2 实现 `backlog/todo/in_progress/in_review/done/blocked/cancelled`、agent/squad Assignee 与 backlog→todo 单次触发
- [x] 8.3 实现 Run List opaque cursor、稳定排序、status/assignee filters 和 Workspace 隔离
- [x] 8.4 实现 Squad create/update/member mutation 的 Agent-only、Invocation Gate、同 Workspace 和恰好一个 Leader 事务不变量
- [x] 8.5 实现 Leader 原子更换以及禁止直接 Remove 当前 Leader 的行为
- [x] 8.6 实现 Squad Archive 的 Active Task 409、禁止新分配、历史 Issue Assignee 保留和无 Restore 行为
- [x] 8.7 将 Comment 收口为 Issue 下 List/Create append-only，可信生成 author/type/source_task/workspace
- [x] 8.8 实现结构化 agent/squad Mention、跨 Workspace 拒绝、持久化防重和 Leader self-trigger guard
- [x] 8.9 实现 Member result 唤醒 Leader、delivered/coalesced comment reconciliation 与运行中补充的 follow-up 收敛
- [x] 8.10 收口 Task Core 的 enqueue、batch claim、prepare lease、waiting_local_directory、deferred promotion、retry 和 orphan recovery
- [x] 8.11 实现 Complete/Fail/Cancel/Session/Usage/Message 的幂等终态和不可回退约束
- [x] 8.12 实现 Issue/Comment/Chat Message 的 Idempotency-Key 与 request hash 防重、不同 Body 409
- [x] 8.13 实现 TaskRun 展开 DTO、Task Messages cursor 和 Flat Comments/Task Runs 独立查询
- [x] 8.14 实现 Workspace、Runtime、Agent、Skill、Chat、Squad、Issue 的冻结 Active/Idle 删除或归档事务语义
- [x] 8.15 实现不依赖 FK/CASCADE 的跨表 Service 校验、事务和锁，并覆盖租户一致性并发测试
- [x] 8.16 实现 Squad Leader Evaluation 的 `action/no_action/failed` 冻结集合、可信 Task 归因与非 Squad Issue 拒绝
- [x] 8.17 删除 Project、Parent、Label、Attachment、Stage、Property、Subscriber、Reaction、Quick Create、Inbox、Channel 和 Autopilot 分支
- [x] 8.18 运行 SQ-01..11、IS-01..11、CM-01..11、FLOW-01..07、RC-01..04、TQ-01..11 和 API-08..10

## 9. Direct Chat Core（Phase 2）

- [x] 9.1 收口 Chat Session create/list/get/patch/delete 与 archived/Active Task 生命周期
- [x] 9.2 实现纯文本 Send Message 创建 Message 和唯一 Chat Task，保证 Task 仅关联 chat_session_id
- [x] 9.3 将 Runtime stream/progress 最终收敛为 Assistant、Failure 或 No-response Message
- [x] 9.4 将用户取消统一为 `/api/tasks/{taskId}/cancel` 并实现 queued/running cancel ack/finalize
- [x] 9.5 实现 Draft Restore create/list/delete 和 deferred cancellation sweeper 收敛
- [x] 9.6 实现 pending task 查询用于刷新后恢复流式展示，无未终态 Task 时返回空结果
- [x] 9.7 保留安全 Session/WorkDir 续轮，删除 Attachment、Pinned/Unread、Quick Action、Project Context 和 Channel History
- [x] 9.8 覆盖 Chat create/send/complete/fail/cancel/resume/archive/idempotency 和 runtime missing contract tests
- [x] 9.9 运行 CH-01..11 并确认 Direct Chat 不依赖 Issue

## 10. Lightweight Web 产品表面（Phase 1 骨架 / Phase 2 对齐）

10.1、10.7、10.11 与 10.13 的 route tree、Run pages、退出深链清理和 API client 属于 Phase 1 gate；其余页面对齐随第 5–9 组的目标 API 在 Phase 2 完成。

- [x] 10.1 建立只包含 Auth、Workspace、Runtime Profile、Skill、Agent、Chat、Squad 和 Run 的 Web route tree、Sidebar 与全局入口
- [x] 10.2 实现 Email Login、Workspace create/select/settings 和只读 Member List 页面与错误状态
- [x] 10.3 对齐 Daemon/Runtime 状态、Runtime Profile 和 Skill/File/Agent Binding 页面到目标 API
- [x] 10.4 对齐 Agent create/edit/archive/restore、权限、Secret redaction 与 Env Reveal/Update 页面
- [x] 10.5 对齐 Direct Chat session/message/stream/cancel/draft restore 页面并移除退出字段
- [x] 10.6 对齐 Agent-only Squad、Leader/role mutation、Archive guard 与 Member status 页面
- [x] 10.7 新建 Lightweight Run List/Create，使用 opaque cursor、status/assignee filter 和 blocked 状态
- [x] 10.8 新建 Run Detail 的 Flat Comments 与 Task Runs 两个独立分区，移除 mixed timeline 与 N+1 display 查询
- [x] 10.9 让 React Query 拥有全部 Server state，Zustand 仅保存筛选/草稿/Modal 等 client state
- [x] 10.10 实现 Web event_id 去重与 reconnect 后目标 query invalidate/refetch
- [x] 10.11 移除 Board/Table/Properties/Labels/Subscribers/Reactions/Calendar/Child/Autopilot/Inbox/Channel/Billing/Cloud/Attachment/Invitation/Builder 页面、深链和快捷入口
- [x] 10.12 将 Picker、Permission Helper、Comment Row、Task Status Pill、Markdown/Avatar 放入符合 package boundary 的共享位置
- [x] 10.13 更新 TypeScript DTO 与 API client snapshot，确认无 next/router 或 react-router 跨包边界回归
- [x] 10.14 运行目标 Web unit/typecheck/build 和浏览器 E2E，覆盖 BLD、Run、Chat、Squad、Realtime 与 removed deep links

## 11. CLI、内置 Skills 与目标契约同步（Phase 1 入口清理 / Phase 2 协议对齐）

- [x] 11.1 保留并对齐 Daemon/Runtime、Agent、Chat、Squad、Issue/Comment/Task、Workspace、Runtime Profile 和 Skill CLI 命令
- [x] 11.2 删除 Project、Label、Property、Attachment、Autopilot、Update 和其他退出 CLI 命令与帮助文本
- [x] 11.3 更新内置 Squad briefing、协作协议和目标 Skills 使用 114 条 manifest 与 Task Token 权限
- [x] 11.4 扫描 Server/Web/CLI/Daemon/内置 Skills 的 path、DTO 和 event 常量，移除旧 alias 与退出字段
- [x] 11.5 运行 CLI contract tests，确认不再静默调用 removed route 或旧 Daemon protocol

## 12. `multica_lightweight` 运行时切换与数据门禁（Phase 3）

- [x] 12.1 在 migrate/Server 的 DDL、监听与写入前实现 PostgreSQL >=15、expected name、edition 和 supported schema version guard
- [x] 12.2 让 composition root 只接受新 schema，并把目标 `migrate` 命令固定为只安装 Lightweight baseline
- [x] 12.3 更新 `.env.example`、Makefile、Compose、ensure/local/worktree env、check、CI 和 migrate 的新库默认配置
- [x] 12.4 实现 worktree test database suffix 与 `make db-reset` 名称 allowlist，确保 CI 不创建/迁移 `multica`
- [x] 12.5 在真实 PostgreSQL 执行并发 Claim、Workspace Delete 与跨表租户一致性验证
- [x] 12.6 执行 Backup/Restore、独立 Secret Store 注入、schema guard、decrypt probe 与 P0 smoke test
- [x] 12.7 对比原 `multica` checksum/revision，确认整个新库验证期间未写入旧库
- [x] 12.8 运行 DB-01..10 与 SEC-12，保存 pg_catalog、migration audit、数据库和 Secret 证据

## 13. 物理删除与依赖清理（Phase 5）

- [x] 13.1 在第 12 组数据库门禁和真实 P0 闭环通过后，删除退出能力的 Handler、Service、middleware 和 integration 源码
- [x] 13.2 删除退出能力的 SQL queries、generated models、backfill commands 和旧 293 migration 启动链
- [x] 13.3 删除退出页面、API client、CLI、内置 Skill、tests、i18n、assets 和不再引用的 shared code
- [x] 13.4 从 Go modules、pnpm workspace packages 和应用 package dependencies 移除仅供退出能力使用的依赖
- [x] 13.5 运行 Go/TypeScript import、SQL/query、path、event、route、worker 和 package dependency 扫描，修复全部目标外引用
- [x] 13.6 重新运行 Router=114、Worker allowlist、26 表和 removed route/deep-link 负向矩阵
- [x] 13.7 在删除后重复真实 PostgreSQL、Daemon/Runtime、Web P0 与安全 P1，确认删除未破坏可靠性或权限
- [x] 13.8 再次比较旧库 checksum/revision，确认 Physical Delete 未触发旧库 migration 或写入

## 14. 量化验收与发布（Phase 4 与 Phase 6）

14.4 至 14.7 是 Phase 4 的真实进程闭环，必须先于 Phase 5 物理删除通过；其余项在 Phase 6 用最终候选版本重跑。

- [x] 14.1 用与基准完全相同的环境和参数构建目标 Server/Web，并保存 commit、工具链、命令、样本与原始 hash
- [x] 14.2 验证 Server binary 不超过基线 70%，Web production JS 不超过基线 60%
- [x] 14.3 在同机同配置多次测量 idle RSS，验证目标不超过基线 80%
- [x] 14.4 用独立进程/端口/HTTP 时间线验证 PostgreSQL Ready 后 Server 在 3 秒内 Ready
- [x] 14.5 用已安装目标 Runtime、独立 Daemon、Register/Token/Heartbeat 时间线验证 5 秒内 Ready
- [x] 14.6 运行真实双 Daemon 并发 Claim、Complete/Fail 重放、WS hint 丢失/重复和 HTTP fallback
- [x] 14.7 用浏览器与真实 Runtime 完成 Agent Direct Chat 和 Leader-Member-Leader 至 `in_review` 闭环
- [x] 14.8 执行 Fresh Checkout `make check`、目标 Web build/typecheck/unit/browser E2E 和全部 Go tests，任何 DB skip 保持发布未通过
- [x] 14.9 完成全部 P0、全部安全 P1、指定 FLOW/CM/RC/TQ、API-01..10、PRC-01..07、BLD-01..05、DB-01..10
- [x] 14.10 生成按 142 个 ID 索引的发布证据包，标注环境、commit、时间、原始日志、未运行和失败项
- [x] 14.11 确认无跨 Workspace 泄露、Leader self-trigger 无限循环或运行期间 Comment 永久丢失
- [x] 14.12 仅在五个 capability specs、全部硬阈值和强制 ID 无 skip 通过后批准 Lightweight release
- [x] 14.13 记录回滚发行物与独立 DSN，将旧 `multica` 和失败候选 `multica_lightweight` 的处置固定为只读保留而非 downgrade/双写
