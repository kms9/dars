# Phase 3：Collaboration Core 退出分支扫描（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 目标 Core 边界

- 唯一 Lightweight composition manifest 只注册冻结的 114 条 API；Project、Parent Issue、Label、Attachment、Stage、Property、Subscriber、Reaction、Quick Create、Inbox、Channel、Autopilot 的路径均不注册，removed-route contract 返回 404。
- `server/internal/lightweightapi` 仅实现 Workspace、Runtime/Profile、Agent/Skill、Chat、Squad、Issue/Comment、Task/Daemon/Token/Realtime 目标能力，不构造退出能力 Service。
- `server/lightweight/queries` 与生成的 `server/pkg/lightweightdb` 只访问 26 表 baseline；机器 contract 显式拒绝 `project/attachment/autopilot/channel/inbox/quick_action/label/property/subscriber/reaction` 等退出表族。
- Lightweight Issue DTO/SQL 不包含 parent issue、project、stage、label、property、subscriber、reaction 或 attachment 字段；Comment DTO 不包含 resolve/thread/reaction/attachment 分支；Chat Send 严格拒绝 `attachments` 字段。
- `agent_task_queue.parent_task_id` 是有限重试/委派 Task lineage，不是已退出的 Parent Issue 产品能力，因此按可靠性协议保留。

## 物理删除边界

本任务完成的是 Phase 2 Lightweight Core 分支删除。Legacy Handler/query/migration/Web/CLI 源码的物理删除仍由 13.1–13.7 在数据库切换、Web/CLI 对齐与真实 P0 门禁之后执行；本阶段不提前删除这些回滚所需旧文件。

## 验证

```text
go test ./lightweight ./internal/lightweightapi ./cmd/server \
  -run 'TestTargetQueries|TestGeneratedModels|TestLightweight' -count=1

Result: PASS
```

Fresh PostgreSQL Live Flow 同时验证 removed Chat attachment 字段返回 400，目标 Core 不创建退出能力表或数据。
