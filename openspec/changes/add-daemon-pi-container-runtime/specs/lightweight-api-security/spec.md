## MODIFIED Requirements

### Requirement: Daemon 使用唯一协议与专用 Token
Daemon 注册 MUST 声明 `lightweight-runtime-v1`，Server 不得降级旧协议。Register SHALL 接受 Human JWT/PAT 首次配对，也 SHALL 接受已由工作区 owner/admin 预配且绑定同一 `workspace_id + daemon_id` 的有效 `ddt_`。Human 配对路径 SHALL 按 `workspace_id + daemon_id + runtime identity` 幂等 upsert 并返回一次可见的 Daemon Token；预配 `ddt_` 路径 SHALL 要求 token 有可解析的 `user_id` owner 种子，幂等 upsert Runtime 但 MUST NOT 轮换、回显或失效调用 token。优雅 Deregister SHALL 只把运行时置为 offline 并保留 token；显式轮换/撤销、工作区删除或过期 SHALL 撤销旧 Token。过期 Token 必须由当前工作区 owner/admin 重新预配，或由 Human 重新配对后才能恢复。

#### Scenario: Human 首次配对与后续请求
- **WHEN** Human JWT/PAT 以正确协议 Register Daemon
- **THEN** Server 签发一次可见的 Daemon Token
- **THEN** 后续 heartbeat、claim 与 lifecycle 请求只能用该 Token 访问绑定范围

#### Scenario: 使用匹配 Daemon Token 注册
- **WHEN** daemon 使用有效预配 `ddt_` 注册其绑定的 Workspace 与 daemon identity
- **THEN** Server 幂等 upsert Runtime、保留当前 token 有效性，并允许该 daemon 继续既有 claim、heartbeat 与 task lifecycle

#### Scenario: 协议错误、scope 错误或 Token 过期
- **WHEN** Daemon 使用非目标协议、被撤销/过期 Token，或 `ddt_` Register 请求声明其他 Workspace/daemon
- **THEN** Server 返回稳定拒绝且不创建 Runtime、不匿名降级、不跨 Workspace

#### Scenario: 优雅停止后复用 token
- **WHEN** Daemon 使用有效预配 `ddt_` 注销其运行时并随后以相同 scope 再次 Register
- **THEN** 注销只把运行时置 offline，token 保持有效且再次注册不产生新的运行时 identity

## ADDED Requirements

### Requirement: 工作区管理员可通过专用 API 预配 Daemon Token
Server SHALL 提供 `GET|POST /api/workspaces/{workspaceId}/daemon-tokens` 与 `DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}`，并使用工作区 admin boundary 创建/轮换、列出非敏感元数据和撤销稳定 daemon identity 的 `ddt_`。Daemon Token 记录 SHALL 保存当前签发/轮换 Human 的 `user_id`、显示名称、工作区、daemon identity、hash、prefix、创建时间和非空有效期；明文 MUST 只在创建/轮换成功响应中出现一次。默认有效期 SHALL 为 90 天且显式有效期 MUST 不超过 365 天。三条专用路由 MUST NOT 改变个人 PAT `/api/tokens` 的 DTO/权限，MUST NOT 扩大 `ddt_` 的 Daemon API 权限。

#### Scenario: Human 管理 Daemon Token
- **WHEN** Workspace Owner/Admin 使用有效 Human 身份预配、轮换或撤销目标 daemon 的 Daemon Token
- **THEN** 明文仅在签发或轮换响应中出现一次，读取只返回元数据，旧值在轮换或撤销后不能再注册或调用 Daemon API，匹配运行时立即变为 offline 但运行时行不被删除

#### Scenario: 无权限 Human 或错误 Workspace
- **WHEN** 普通 Member 或其他 Workspace Human 尝试预配、列出或撤销目标 Daemon Token
- **THEN** Server 返回 403/404，不泄露 token 元数据且不改变现有 token

#### Scenario: 签发人离开工作区
- **WHEN** 原签发 Human 离开工作区但 token 仍有效，随后当前 owner/admin 管理该 token
- **THEN** `ddt_` 继续作为工作区机器凭据工作，当前 owner/admin 仍可列出、轮换或撤销；轮换把 `user_id` 更新为当前 actor，token 使用不要求原签发人仍是成员

#### Scenario: 旧 token 缺少 owner 种子
- **WHEN** 升级前孤儿/多 owner token 无法唯一回填 `user_id` 并尝试 `ddt_` Register
- **THEN** Server 拒绝直接注册且不创建运行时，当前 owner/admin 可通过轮换写入新 `user_id` 后恢复

### Requirement: 容器 bootstrap 与任务 credential 必须隔离
Daemon SHALL 从 `DARS_DAEMON_TOKEN_FILE` 或 `DARS_DAEMON_TOKEN` 读取运行期 credential，但 MUST 在启动 Pi、ACP model discovery 或任何任务子进程前从子进程环境中移除 token 值和所有继承的 `DARS_*`。Human PAT/JWT 与其他 `ddt_` 同样 MUST NOT 作为 `DARS_TOKEN` 或其他变量泄露给任务；任务子进程只能获得 Server 为当前 Task 签发的 `dat_`、运行目标所需的 Provider credential 及显式授权的 Agent execution secrets。

#### Scenario: 检查 Pi 任务环境
- **WHEN** 容器通过预配 `ddt_` 注册并执行一个能检查环境变量名称的 Pi 任务
- **THEN** 任务仅观察到当前 `dat_` 形式的 `DARS_TOKEN`，观察不到 `DARS_DAEMON_TOKEN`、任何 `ddt_` 或 Human credential

#### Scenario: 日志和错误路径
- **WHEN** token provisioning、registration、heartbeat 或任务执行产生成功或错误日志
- **THEN** 日志不包含运行期 Daemon、Human、Task 或 Provider credential 的完整值或可恢复片段

#### Scenario: 非 task ACP 探测环境
- **WHEN** 守护进程为 runtime discovery 启动 ACP model probe
- **THEN** probe 环境同样不继承 `DARS_DAEMON_TOKEN` 或其他守护进程 `DARS_*` 值

### Requirement: Router 精确匹配批准的 145 条 manifest
Server SHALL 只注册 checked-in route manifest 中按 `HTTP method + path template` 归一化后的 145 条路由。目标 manifest SHALL 以当前活跃 142 条路由为基线，只增加下列 3 条工作区级 Daemon Token 管理路由，并且不得注册重复路径、旧 Desktop alias 或其他未批准业务路由；query string 不计入 path template。

```http
GET /api/workspaces/{workspaceId}/daemon-tokens
POST /api/workspaces/{workspaceId}/daemon-tokens
DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}
```

#### Scenario: Router snapshot 精确匹配
- **WHEN** contract test dump 全部已注册路由并按 method+path 排序
- **THEN** 结果与 checked-in 145 条 manifest 精确相等、无重复且包含上述 3 条 delta
- **THEN** 既有 `/api/tokens` PAT、Skills、Tool Gateway、Agent/Squad 和 Daemon lifecycle 路由保持不变

#### Scenario: 未批准路由不可达
- **WHEN** 客户端访问其他 Daemon Token alias、旧 Desktop alias 或任意未列出路径
- **THEN** Server 返回 404 且不进入旧 Handler

## REMOVED Requirements

### Requirement: Router 精确匹配批准的 126 条 manifest
**Reason**: 126 是恢复 Agent/Squad 后的旧历史数字，当前权威代码与 checked-in route list 已为 142；本变更再批准 3 条工作区级 Daemon Token 管理路由。

**Migration**: 以当前 142 条活跃 manifest 为基线生成并校验 145 条目标 manifest，同时更新 route metadata 和相关发布断言。
