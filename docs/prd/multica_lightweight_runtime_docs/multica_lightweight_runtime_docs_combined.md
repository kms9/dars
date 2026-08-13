# Multica 轻量智能体与小队协作运行时文档导航

> 基础仓库：`kms9/multica`  
> 基准提交：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`  
> 校准日期：`2026-08-04`  
> 目标数据库：`multica_lightweight`  
> 文档状态：`P0 需求已冻结 / 可进入 OpenSpec proposal 与 design / 目标待实现`

## 1. 使用说明

为避免“合集副本”与分册正文发生漂移，本文件不再复制 00–04 的全文。以下分册是唯一正文来源：

1. [00_multica_lightweight_runtime_prd.md](./00_multica_lightweight_runtime_prd.md)：产品目标、当前边界、状态模型、冻结决策与 Definition of Ready；
2. [01_code_scope_checklist.md](./01_code_scope_checklist.md)：代码保留/裁剪/停用/删除范围及阶段门槛；
3. [02_api_reduction_spec.md](./02_api_reduction_spec.md)：当前 canonical API、最终目标 manifest、Token 权限与 Realtime 契约；
4. [03_database_retention_spec.md](./03_database_retention_spec.md)：26 表字段 baseline、约束、索引与数据库身份保护；
5. [04_e2e_acceptance_matrix.md](./04_e2e_acceptance_matrix.md)：业务、安全、API、进程、迁移和轻量化量化验收。

## 2. 校准后的共同结论

- 当前仓库仍是完整产品，不存在可直接整体删除的独立 Lightweight Runtime 模块；
- Agent、Chat、Squad、Issue/Comment Trigger、Durable Task Queue 已具备目标闭环基础；
- 当前 Router、Handler、TaskService、IssueService、前端壳层和数据库仍深度耦合外围能力；
- 第一阶段只允许收缩页面、导航、目标 Router 和后台 Worker，不立即 Drop Table 或重写历史 migration；
- `deferred`、`waiting_local_directory`、prepare lease、comment reconciliation、task token、invocation permission 和安全审计属于当前核心可靠性/安全协议；
- 当前仓库直接替换为单一轻量产品，不维护 Full/Light 双模式；
- 不兼容旧 API、旧 Desktop 或原完整产品数据库；
- 当前阶段不修改和验收 Desktop 专属代码；
- 根 TypeScript 构建、CI 和 `make check` 必须排除 Desktop/Mobile/Docs，只验证 Web 与目标共享包；
- 新数据库默认名为 `multica_lightweight`，从空库创建精简 baseline，不回放 293 个旧 up migrations；
- P0 只支持 Local Runtime，保留 `blocked`，不支持 Attachment、Human Squad Member 或 Workspace Invitation；
- Run Detail 固定为 Flat Comments 与 Task Runs 两个分区，不提供混合 `/timeline`；
- Squad 只允许 Agent Member，存在 Active Task 时禁止归档，历史 Issue Assignee 不转移；
- 原完整产品数据库不得被目标服务写入或复用，只作为只读归档保留；
- 最终完成标准包括真实 PostgreSQL、独立 Daemon/Runtime、Web、Fresh Install、Backup/Restore 与量化规模证据，不以页面减少或单元测试自述代替。

## 3. 当前 canonical 契约摘要

- Agent：`PUT /api/agents/{id}`，`POST /archive`，`POST /restore`；
- Squad：`PUT /api/squads/{id}`，`DELETE` 为当前归档/清理入口，无 Restore API；
- Issue：`GET/POST /api/issues`，`GET/PUT /api/issues/{id}`，任务列表为 `/task-runs`；
- Comment：Issue 下 List/Create；当前没有单 Comment GET；
- Chat Cancel：用户通过 `POST /api/tasks/{taskId}/cancel`，不是 Session-scoped Cancel；
- Daemon 批量 Claim：canonical 为 `POST /api/daemon/tasks/claim`，`/api/daemon/claim` 是兼容 alias。

以上只描述基准提交现状。目标契约以 API 分册第 18 节为唯一权威来源；两者冲突时不得用当前 Router 覆盖目标设计。

## 4. 已冻结决策

1. 当前复制仓库直接替换为单一轻量版；
2. 不提供旧 API/Desktop/数据库兼容；
3. 当前阶段排除 Desktop 专属修改；
4. 使用新数据库 `multica_lightweight` 和独立精简 baseline；
5. 旧数据库不迁移、不覆盖、不作为运行 DSN；
6. P0 只支持 Web + Server + Local Daemon/CLI；
7. 保留最小 Workspace/Member Read-only、Runtime Profile、Skill 和 Agent-Skill 页面；
8. 不保留 Attachment、Human Squad Member、Workspace Invitation、Cloud Runtime 或混合 Timeline；
9. 保留 `blocked`，Run List 使用 opaque cursor；
10. 最终 API/Token/Realtime/Worker 白名单、26 表 baseline 和量化阈值均以各分册冻结值为准。

本版没有阻塞 P0 实施拆分的产品开放项。任何新增范围或阈值调整都必须另立 OpenSpec change，不能在实现中隐式恢复完整产品能力。

## 5. 推荐实施顺序

```text
契约冻结
→ 页面/导航/API/Worker 表层收缩
→ Handler/Service/Core 解耦
→ 新建 multica_lightweight baseline + sqlc
→ 真实 PostgreSQL + Daemon + Web 验证
→ 物理删除模块、查询、依赖和表
→ Fresh Install/Database Guard/Backup-Restore/Release 验收
```

## 6. 实施期主要风险与强制控制

| 风险 | 强制控制 |
|---|---|
| 共享 Core/View 裁剪导致 Desktop 编译失败 | Desktop 从目标构建和 required checks 排除，不添加兼容层 |
| 旧 Handler/SQL/Worker 仍被间接构造 | Router dump、Worker allowlist、import/query 扫描和 removed-route 404 合并验收 |
| 新 baseline 漏掉可靠性字段或索引 | 以数据库分册 1.3、17、20 节生成 DDL/sqlc，并跑真实 PostgreSQL 并发测试 |
| Daemon Token 中间件存在但签发主流程未接通 | 将 Register 配对、一次性 Token 返回、重配撤销、过期拒绝作为独立 P0 任务和 DR-10/11 验收 |
| 当前 Agent Env/MCP 只做响应脱敏、库内仍是原始 JSON | 引入版本化密文 envelope、生产密钥 readiness、DB/日志探针和备份恢复解密测试 |
| 误连或误删原 `multica` 数据库 | migrate/server name-edition-version guard、reset allowlist、旧库 checksum/revision 对比 |
| breaking change 在 Web/CLI/Daemon 之间漂移 | API 第 18 节作为单一 manifest，四端同批切换并跑 contract tests |
| 页面减少但产物/RSS 没有真正变轻 | 二进制、JS、RSS、启动耗时、Worker、路由和表数量全部作为发布硬门槛 |

当前结论是“需求边界已具备可实施性”，不是“工程已经完成”。下一步应创建 OpenSpec proposal/design，将上述契约拆成可回滚的 Surface、Core Extraction、New Baseline、Physical Delete 和 Release 任务。
