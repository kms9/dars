# Phase 3：Mention 与 Leader re-entry（阶段证据）

记录时间：2026-08-05（Asia/Shanghai）

## 已实现协议

- 只解析 Markdown 结构化 `mention://agent/<uuid>` 与 `mention://squad/<uuid>`；普通 `@name`、member/issue/all mention 不触发 Task。
- 所有显式目标必须能在当前 Workspace 内解析；跨 Workspace UUID 使用不泄露目标身份的 `400 mention_target_not_found` 拒绝，Comment 与触发 Task 在同一事务回滚。
- Agent 与 Squad Mention 复用 private/public_to Invocation Gate。Agent 发起的委派沿用来源 Task 的 originator/accountable/initiator；缺少历史 attribution 时只回退到来源 Agent Owner，不信任请求字段。
- Squad Mention 只解析并触发唯一 Leader。Leader 对自身 Agent 或自身 Squad 的 Mention 被 self-trigger guard 抑制。
- 来源 Leader Task 显式 Mention 同 Squad Member 时，新 Task 保存 `squad_id` 且 `is_leader_task=false`；Member 结果 Comment 无需显式 Mention 即重新路由至该 Squad 唯一 Leader。
- 同一个 Comment 内重复 Mention 先去重；同一 Agent 已有 Active Task 时不创建第二条，而是把 Comment ID 原子追加到该 Task 的 `coalesced_comment_ids`。
- Comment 持久化、trigger/coalesce 与 Issue `updated_at` 在同一 Issue 行锁事务中完成；Idempotency replay 不重新执行 routing，形成持久化防重边界。
- Daemon Claim 在发放 Task Token 前，把当次真正计划交付的 `trigger_comment_id + coalesced_comment_ids` 固化为 `delivered_comment_ids`，并在响应 `comment_ids` 中返回同一集合。
- Claim 后到达的 Comment 只进入 planned/coalesced，不伪造 delivered receipt。Complete 与 Comment 都先锁同一 Issue；Complete 将 planned-but-undelivered 集合收敛为单个 follow-up Task。
- Member result 如果到达时 Leader follow-up 已 queued，则原子合并到该 follow-up；下一次 Leader Claim 同时收到用户补充与 Member 结果，不会丢失或重复执行。

## PostgreSQL 16 真实流程

环境：全新隔离本地 PostgreSQL 16.13，数据库名 `multica_lightweight`，从空库应用全部 77 个 Lightweight migrations。远程 PostgreSQL 未创建数据库、未迁移、未写入。

```text
LIGHTWEIGHT_DATABASE_URL=<temporary-local-dsn> \
  go test ./internal/lightweightapi \
  -run '^TestLightweightIdentityWorkspaceTokenLiveFlow$' -count=1 -v

Result: PASS
```

真实流程覆盖：

- Leader 的普通 `@resource-agent` 文本不触发；同一 Comment 内两次 `mention://agent` 只生成一个 Member Task，idempotent replay 后 Task 数仍为一。
- Leader `mention://squad/<own-squad>` 后 Active Leader Task 数不增加。
- 指向另一 Workspace Agent 的结构化 Mention 返回 400，Comment 行数不变。
- Leader Task 运行期间用户新增 Comment：该 ID 进入 planned 而非 delivered；Leader Complete 后生成一个 Leader follow-up。
- Member Task Claim 携带委派 Comment ID；Member Complete 写入可信 `source_task_id` 结果 Comment，并把 Leader 重新唤醒。
- Leader follow-up Claim 同时携带运行期用户 Comment 与 Member result Comment；完成后 Active Task 归零。
- 独立竞态用例让 Human Comment 与 Daemon Complete 同时进入：两个请求均成功，数据库恰好一个 Active follow-up，下一 Claim 必然包含竞态 Comment ID，follow-up 完成后 Active Task 为零。

## 尚未宣称完成

- 本证据完成 8.8 与 8.9，并覆盖 CM-02..08、FLOW-01 与 TQ-11 的服务端核心语义；完整浏览器/CLI acceptance matrix 仍留在 8.18、11.x 与 14.x。
- 多成员并行、两阶段委派、Member failure 展示等完整 FLOW-02..07 仍需后续矩阵验证。
