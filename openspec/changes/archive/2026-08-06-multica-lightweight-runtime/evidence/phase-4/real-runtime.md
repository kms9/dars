# Phase 4：真实进程与 Runtime 闭环证据

验证窗口：2026-08-06 10:11–11:28 Asia/Shanghai。

## 环境

- macOS 15.2 arm64；Go 1.26.1；Node 24.16.0；pnpm 10.28.2。
- Docker PostgreSQL 17，loopback `127.0.0.1:5432`，持久化数据库 `multica_lightweight`。
- Lightweight Server `127.0.0.1:28080`；Web `localhost:23000`。
- 两个独立 Daemon：`019db5c7-95bf-7470-bdc5-64a994620a74` 与 `019dccb5-1b4e-7d62-bf60-ef3c76d29a02`。
- 两个独立 Codex Runtime：`ae908058-c88d-470b-9be2-3a7a2813a040` 与 `cf86c350-fcdc-4dd5-8de1-8aab007ae8de`。
- App 内置 Browser 当前无可用 Runtime；按 Browser skill 的 fallback 边界使用本仓库 Playwright/Chromium 进行同等真实页面验证。

所有 Secret 只来自忽略提交的 `.env` 或 CLI profile；证据、命令输出和仓库文件不包含明文。

## 启动门槛

最终候选二进制在 PostgreSQL 已 Ready 后独立启动：

```text
server_ready_status=200
server_health_status=200
server_ready_seconds=0.501632

daemon_status=running
daemon_ready_seconds=1.895808
daemon_agent_provider_count=6
daemon_workspace_count=1
```

分别满足 Server `<=3s`、Daemon `<=5s`。Daemon Health 在 preflight、首次 Workspace sync、Runtime register 完成前保持 `starting`，本次只以 `running` 作为 Ready。

## Direct Chat 与 Streaming

浏览器使用 Email Code 完成真实登录，进入 Chat Session `944ea002-de9a-4b46-bd18-e5c36614cd70`，通过真实 Codex Runtime 执行 Task `d5d689ed-2f1f-462e-94d6-3a326a4f2070`。

浏览器观测：

```text
task-message GET status=200 (连续轮询)
stream_visible=STREAM_PHASE_ONE
final_assistant=STREAM_PHASE_TWO
stream_hidden_after_completion=true
browser_console_errors=0
```

数据库序列：

```text
1 text        STREAM_PHASE_ONE
2 tool_use    exec_command
3 tool_result exec_command
4 tool_use    exec_command
5 tool_result exec_command
6 text        STREAM_PHASE_TWO
```

Web 只渲染 `type=text`；`thinking`、tool input 与 tool output 不进入页面。Task 完成后 streaming article 消失，最终 Assistant Message 只持久化一次。`packages/views/lightweight/chat.test.tsx` 另以乱序输入验证按 `seq` 排序，并证明 private reasoning/tool output 不可见。

## FLOW-01：双 Runtime 单成员闭环

Run `LOC-3` / `77df6321-e258-4f9c-afdb-42bb4d431291`：

```text
Leader 074423b2... @ primary  -> completed
Member 763aa851... @ secondary -> completed
Leader re-entry @ primary       -> completed
Run status                       -> in_review
Leader evaluation                -> action, then no_action
Member result projection count   -> 1
```

Comment 顺序为 Leader canonical delegation、Member completion projection、首轮 Leader completion projection、Leader re-entry completion projection。re-entry 没有重新委派，未产生 Leader self-trigger 循环。

## FLOW-02：真实顺序二阶段闭环

Run `LOC-4` / `6d7c28ce-4f32-4c43-a60b-f7c895a3a545`，Squad `33be28ff-e038-477e-af33-430c4c233c91`：

```text
1 Flow2 Leader  @ primary   completed
2 Flow2 Backend @ secondary completed
3 Flow2 Leader  @ primary   completed
4 Flow2 Test    @ secondary completed
5 Flow2 Leader  @ primary   completed
final Run status             in_review
Backend projection count     1
Test projection count        1
task failures                0
```

持久化 Comment 严格按以下顺序出现：Backend canonical mention、Leader stage-one projection、`BACKEND_STAGE_OK`、Test canonical mention、Leader stage-two projection、`TEST_STAGE_OK`、`FLOW2_LEADER_OK`。Test Task 只在 Backend 结果落库后创建，没有并行越序或重复阶段。

## 可靠性边界

- 双 Daemon 在同一真实 Server 上注册、Heartbeat、WS wakeup、Claim 与 Complete；两个 Runtime 均执行过真实 Task。
- `make check` 中的真实 PostgreSQL 测试覆盖并发 Claim 单赢家、Complete/Fail 重放、WS 结果不确定、丢失/重复 hint 与 HTTP fallback。
- 真实 FLOW-01/FLOW-02 证明 Member completion projection 只写一次、Leader re-entry 不自触发、运行期结果不会永久丢失。

