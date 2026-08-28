# Dockerfile.daemon-pi 需求文档

> 状态：Implemented v1.0（架构评审、实现与真实 Pi 黑盒验收已完成）
> 目标版本：DARS 默认可部署 Agent Runtime 节点  
> 范围：`Dockerfile.daemon-pi`、容器启动入口、Daemon Token 预配与非交互认证、Pi 运行时自检、DARS 连接与 Runtime 注册、端到端任务验收

## 1. 背景

当前 DARS daemon 已经具备 Pi backend，能够发现 `pi` CLI，并通过非交互 JSON 模式执行任务；但当前仍缺少一个“开箱即用”的默认 Runtime 发行形态。

目标是提供一个独立的 `Dockerfile.daemon-pi`，将：

- DARS CLI / daemon
- Pi Coding Agent Runtime
- Pi 运行所需 Node.js Runtime
- Git / Bash / ripgrep / CA certificates / SSH client 等基础执行依赖
- 容器启动、自检和健康检查逻辑

打包为一个可直接运行的 DARS Execution Node。

用户只需要提供：

1. DARS 远程 Server 地址；
2. Daemon Token；
3. Workspace / Runtime 必要标识；
4. Pi 模型及 Provider Credential，或一个已经存在的 Pi config 目录；

即可自动启动一个 DARS 可发现、可调度、可执行任务并回传结果的 Pi Runtime 节点。

---

## 1.1 可行性结论与已确认约束

结论：方案可行，但必须同时修改 Server、CLI/守护进程和发行物，不能只增加一个 Dockerfile。现有 Pi backend、Runtime 注册、WebSocket claim、task-scoped `dat_` 和结果回传链路可以复用；原草案中的认证与重启语义不足以支撑无人值守容器。

本轮代码核查和 Grok/Codex 交叉评审确认：

| 项目 | 当前事实 | 最终处理 |
|---|---|---|
| Daemon Token | Human 注册会签发 30 天 `ddt_`，优雅 Deregister 会删除 Token | 增加 Workspace-admin 专用预配 API；默认 90 天、上限 365 天；Deregister 只下线 Runtime，显式 rotate/revoke 才吊销 |
| Register | 当前只接受 Human JWT/PAT | 接受 Human 或 scope 完全匹配且 owner 已解析的 `ddt_`；后者不轮换 Token |
| 容器认证 | 当前仅从 `~/.dars/config.json` 读取 | 统一为 token file > token env > 本地 profile；file 设置即权威，任何错误均 fail closed |
| 路由/schema 基线 | 实际为 142 条路由、35 张表；旧文档的 126/27 已过期 | 新增 3 条专用路由，目标精确为 145；只给现有表加列，仍为 35 张表 |
| Pi 版本 | 本机 Pi 为 0.84.2，要求 Node `>=22.19.0` | 固定 Pi 0.84.2 与满足 engine 的精确 Node 22 patch |
| Pi config | 实际可复用目录为 `/Users/logo/.pi/agent` | 挂载到 `/config/pi`；OAuth 默认可写，只读时 basic 使用 `--no-refresh` |
| Ready | 当前 health 只有 Runtime ID | 增加兼容的 Runtime provider 映射，由容器 health 校验目标工作区中已注册 Pi Runtime |
| E2E | 仅 build 或最终文本不足以验收 | 隔离数据库与空仓库，验证真实 tool-result、唯一 completed 行、精确最终输出、重启/重连/失败恢复 |

安全边界保持不变：`ddt_` 只用于节点到 Control Plane，任务子进程只获得 task-scoped `dat_`；不会把用户 Pi 配置复制进镜像，也不会在日志或验收证据中暴露 Secret。

---

## 2. 核心目标

`Dockerfile.daemon-pi` 不只是“安装了 daemon 和 Pi 的 Docker 镜像”，而应形成一个具有完整启动契约的 DARS Runtime Node。

完整成功链路必须是：

```text
Container Start
    ↓
加载环境变量与挂载配置
    ↓
Pi Runtime Preflight
    ↓
Pi Ready
    ↓
DARS Server Auth / Connectivity Preflight
    ↓
DARS daemon 自动启动
    ↓
Runtime 注册到 DARS
    ↓
DARS 可发现 Runtime
    ↓
DARS 下发 Task
    ↓
daemon claim Task
    ↓
spawn Pi 执行
    ↓
Pi 完成任务
    ↓
daemon 提交消息 / 状态 / 最终结果
    ↓
DARS Task = completed
```

只有完整通过上述链路，才视为 `daemon-pi` 节点真正 Ready。

---

## 3. 非目标

第一阶段不要求：

- 将 Pi 改造为长期驻留的 RPC Runtime Pool；
- 在 daemon 内嵌 Pi SDK；
- 支持多个 Pi 进程常驻池化；
- 在同一个镜像中同时打包 Claude Code、Codex 等其他 Runtime；
- 解决任意不可信代码的强隔离问题；
- 替代未来独立的 Sandbox / Runtime Pool 层。

第一阶段继续复用 DARS 当前 Pi backend 的执行方式：

```text
daemon
  ↓
收到 Task
  ↓
spawn pi -p --mode json ...
  ↓
消费 JSON event stream
  ↓
提交结果
```

---

## 4. 镜像定义

### 4.1 文件

新增：

```text
Dockerfile.daemon-pi
```

建议同时新增：

```text
docker/daemon-pi-entrypoint.sh
docker-compose.daemon-pi.yml
```

### 4.2 镜像内最小组件

必须包含：

- `dars` CLI binary；
- `pi` CLI；
- Node.js Runtime；
- `bash`；
- `git`；
- `ripgrep`；
- `ca-certificates`；
- `openssh-client`；
- `tini` 或等价 PID 1 / signal forwarding 机制。

推荐以非 root 用户运行。

第一阶段发行基线固定为：

- Go builder：仓库声明的 Go 1.26.1；
- Node.js：满足 Pi engine `>=22.19.0` 的精确 Node 22 patch，不使用浮动 `node:22`；
- Pi：`@earendil-works/pi-coding-agent@0.84.2`，允许通过 `PI_VERSION` build arg 显式覆盖；
- Debian slim runtime，npm lifecycle scripts 禁用，构建时断言 Node/Pi 版本并清理缓存；
- OCI labels 记录 DARS/Pi 版本，`tini` 负责 PID 1 与 signal forwarding。

镜像必须支持 build-time UID/GID，使非 root `dars` 用户可读取 mode-0600 的 Pi config 并写入状态 volume。不得把宿主机 Pi 安装或 `/Users/logo/.pi` 复制进镜像。

---

## 5. 配置输入

节点必须同时支持两类配置方式：

### 5.1 环境变量模式

适用于 CI、云主机、Docker / Compose / Kubernetes 等机器部署场景。

#### DARS 配置

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `DARS_SERVER_URL` | 是 | 无 | 远程 DARS Server 地址 |
| `DARS_DAEMON_TOKEN_FILE` | 是* | 无 | 首选的 Docker/Kubernetes Secret 文件；内容必须是 `ddt_` |
| `DARS_DAEMON_TOKEN` | 是* | 无 | daemon 连接 Control Plane 的 Workspace 机器凭据；内容必须是 `ddt_` |
| `DARS_WORKSPACE_ID` | 是 | 无 | Token 所属的唯一工作区 |
| `DARS_DAEMON_ID` | 是 | 无 | Token 绑定的稳定节点 ID；本地 profile 流程仍可自动持久化 |
| `DARS_DAEMON_DEVICE_NAME` | 否 | hostname | DARS 页面显示的设备名 |
| `DARS_AGENT_RUNTIME_NAME` | 否 | `DARS Pi Runtime` | Runtime 显示名 |
| `DARS_DAEMON_MAX_CONCURRENT_TASKS` | 否 | DARS 默认值 | 最大并发任务数 |
| `DARS_WORKSPACES_ROOT` | 否 | `/workspaces` | Docker 内任务工作区 |

`*` 表示 file/env 二选一。容器模式必须提供预配 `ddt_`；只有现有本地守护进程流程允许回退到 profile 中的 Human JWT/PAT 并执行配对。

认证优先级必须为：

```text
DARS_DAEMON_TOKEN_FILE contents
    >
DARS_DAEMON_TOKEN
    >
~/.dars/config.json -> Human JWT/PAT（仅本地流程）
    >
startup error
```

`DARS_DAEMON_TOKEN_FILE` 一旦设置就是权威输入；文件不存在、不可读、为空或不是 `ddt_` 时必须立即失败，不得回退到环境变量或 profile。`DARS_WORKSPACE_ID` 环境变量覆盖 profile 工作区。

Docker / Server 部署不得要求人工执行 `dars login`。

### 5.2 Pi 配置

| 环境变量 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `DARS_PI_PATH` | 否 | `/usr/local/bin/pi` | Pi executable |
| `DARS_PI_MODEL` | 条件必填 | Pi config 默认模型 | DARS 默认调用模型，建议使用 `provider/model` |
| `PI_CODING_AGENT_DIR` | 否 | `/config/pi` | Pi 配置目录 |
| `DARS_PI_PREFLIGHT` | 否 | `basic` | `basic` / `active` / `disabled` |
| Provider API Key | 条件必填 | 无 | 如 `OPENAI_API_KEY`、`ANTHROPIC_API_KEY` 等 |

启动后必须能够解析出一个实际可执行的 Provider + Model。若环境变量和挂载 config 均无法解析出有效模型，应启动失败。

---

## 6. 已存在 Config 目录加载

### 6.1 Pi Config

必须支持将宿主机已有 Pi config 挂载到容器，例如：

```text
host ~/.pi/agent
    ↓ bind mount
/config/pi
    ↓
PI_CODING_AGENT_DIR=/config/pi
```

挂载目录可包含：

```text
settings.json
models.json
auth.json
extensions/
skills/
prompts/
AGENTS.md
...
```

挂载后 Pi 必须按其原生规则使用这些配置。

OAuth 配置默认以可写方式挂载，使 Pi 能正常刷新凭据。只读挂载仅适用于无需刷新的 API Key 或仍在有效期内的 OAuth；basic preflight 必须使用 `--no-refresh`，失效时输出要求改为可写挂载的诊断。容器以非 root UID/GID 运行，入口必须在读取 Secret 前明确报告目录权限错误。

### 6.2 DARS Config / State

必须支持持久化：

```text
/home/dars/.dars
```

其中至少包括：

- daemon identity；
- DARS config；
- daemon logs；
- Pi session files；
- 其他 daemon 本地状态。

容器重启后，在使用同一 volume 的情况下：

- daemon identity 不应无故变化；
- 已有 DARS config 可以继续加载；
- 可恢复的 Pi session 不应因为容器重建直接丢失。
- 同一个未过期、未撤销的 `ddt_` 可以重新 Register，不要求 Human 重新登录或重新配对。

---

## 7. 建议 Volume 边界

```text
/config/pi
    Pi config / auth / models / skills / extensions

/home/dars/.dars
    DARS daemon identity / state / logs / Pi sessions

/workspaces
    DARS Task execution workspaces
```

三类状态应分离，避免把模型凭据、daemon 身份和任务工作区混在同一个 volume 中。

---

## 8. 启动顺序

容器 ENTRYPOINT 必须严格按照下列顺序执行。

### Stage 0：Configuration Validation

检查：

- `DARS_SERVER_URL` 存在且可解析；
- daemon token 可以从环境变量或 config 解析；
- Pi executable 路径存在；
- Pi config 目录存在或允许使用纯环境变量配置；
- workspaces 目录可创建 / 可写；
- DARS state 目录可创建 / 可写。

失败时：

- 容器退出非 0；
- 不启动 daemon；
- 不向 DARS 注册 Runtime。

### Stage 1：Pi Runtime Preflight

Pi 必须先通过自检，再允许 daemon 启动。

#### basic 模式

必须：

1. 验证 `pi` executable 与固定版本可执行；
2. 按 `DARS_PI_MODEL`，否则按 `settings.json` 的 `defaultProvider/defaultModel` 解析唯一 `provider/model`；
3. 在完整 `pi --list-models` catalog 中精确匹配；
4. 对内建 Provider 执行 `pi auth check --model <provider/model> --json` 并只接受 `status:"ready"`；
5. 不读取或打印 credential；只读 config 追加 `--no-refresh`；
6. custom/extension Provider 因 auth-check 不加载扩展 catalog，必须给出“使用 active”的诊断，不得误报 credential 错误。

#### active 模式

在 basic 基础上，额外执行一次真实最小模型调用，以验证：

- DNS；
- Provider endpoint；
- credential；
- model availability；
- Pi Agent Loop 可以正常启动并返回结果。

active probe 必须在新建临时空目录中运行固定低 token 请求，使用 `--no-session --no-tools --no-context-files --mode json`，关闭 stdin，但保留 extension discovery 以支持明确挂载的 custom Provider。成功流必须以 `DARS_PI_PREFLIGHT_OK` sentinel 结束，不保留 session 或工作区。`disabled` 仅在显式配置时接受。

Pi Preflight 失败时：

```text
Pi NOT READY
    ↓
Docker container startup failed
    ↓
DARS daemon MUST NOT start
    ↓
Runtime MUST NOT register
```

### Stage 2：DARS Connectivity / Auth Preflight

Pi Ready 后，再检查 DARS Control Plane。

至少验证：

1. `DARS_SERVER_URL` 网络可达；
2. 解析出的 `ddt_` 可以完成认证；
3. `GET /api/daemon/workspaces` 只返回并匹配 `DARS_WORKSPACE_ID`；
4. daemon 后续所需 HTTP / WebSocket 连接具备建立条件。

Auth preflight 必须是非变更操作，不得注册 Runtime。`/api/me` 只接受 Human 凭据，禁止用于该检查。

失败时：

- 容器不得进入 Ready；
- daemon 不应产生“在线 Runtime”残留；
- 日志必须明确区分 network failure / auth failure / workspace failure。

### Stage 3：启动 DARS daemon

所有前置检查完成后：

```bash
dars daemon start --foreground
```

必须作为容器主进程运行，并正确接收 SIGTERM / SIGINT。

禁止要求人工进入容器执行：

```bash
dars login
```

### Stage 4：Runtime Registration

Daemon 启动后必须：

- 自动发现镜像内 `pi`；
- 将 Pi Runtime 注册到 DARS；
- 周期性 heartbeat；
- DARS Server / Web 可以看到该 Runtime；
- Runtime 状态为在线 / 可调度。

只有 Runtime 注册成功，节点才可被视为真正 Ready。

---

## 9. Token 安全边界

必须区分：

### `DARS_DAEMON_TOKEN`

用途：

```text
daemon → DARS Control Plane
```

属于 Workspace 机器凭据，格式为 `ddt_`。它通过下列 Workspace-admin 专用接口预配和管理：

```http
GET /api/workspaces/{workspaceId}/daemon-tokens
POST /api/workspaces/{workspaceId}/daemon-tokens
DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}
```

POST 使用 `{daemon_id,name,expires_at}`，默认有效期 90 天、最大 365 天；同一 Workspace/daemon 再次 POST 是显式 rotation，只返回一次新明文，并立即使旧 Token 失效且将对应 Runtime 标记 offline。DELETE 显式撤销并下线对应 Runtime。当前 Workspace owner/admin 可以管理机器凭据；原签发人离开不自动使其失效，rotation 将 runtime owner seed 转移给当前操作者。

预配 `ddt_` 调用 Register 时必须同时匹配 Workspace ID 和 daemon ID，且其 owner 已解析；Register 只 upsert Runtime，不轮换也不返回 Token。优雅 Deregister 只将 Runtime 标记 offline，不删除 Token，因此同一节点可以使用相同 volume/Token 重启。Token 到期、Workspace 删除或显式 rotate/revoke 才使凭据失效。

### `DARS_TOKEN`

用途：

```text
DARS Server
    ↓ task-scoped credential
Daemon
    ↓
Pi task process
```

属于任务级短生命周期 Token。

不得将 `DARS_DAEMON_TOKEN` 作为 `DARS_TOKEN` 注入 Pi Task 进程。

不得因为 Docker 化而破坏 DARS 当前的 task-scoped credential 权限边界。

所有任务子进程和 ACP model discovery 都必须从移除全部继承 `DARS_*` 的环境开始，再按协议注入 task-scoped 值；Provider credential 可以保留，因为这是 Pi 支持的显式认证路径。`Client.SetToken(ddt_)` 必须清除陈旧 Human pairing token，避免挂载的旧 profile 静默轮换环境/文件 Token。

---

## 10. 健康状态模型

建议至少区分以下状态：

```text
STARTING
    ↓
PI_READY
    ↓
DARS_CONNECTED
    ↓
RUNTIME_REGISTERED
    ↓
READY
```

其中：

- Docker process 存活 != Ready；
- Pi executable 存在 != Ready；
- daemon 进程存在 != Ready；
- 只有 Pi 可调用、DARS 已认证、Runtime 已注册后才是 Ready。

守护进程本地 health response 在保留 `workspaces[].runtimes []string` 的同时，增加非敏感的 runtime ID→provider 映射。`dars daemon container-health` 只有在下列条件全部成立时返回成功：

- health status 为 `running`；
- 存在 `DARS_WORKSPACE_ID` 对应工作区；
- 该工作区至少注册一个 provider 为 `pi` 的 Runtime。

Docker `HEALTHCHECK` 使用该命令并配置覆盖 active preflight 与首次注册的 `start_period`，不得只检查 PID，也不要求暴露 loopback health port。

---

# 11. 验收条件

以下为第一阶段必须全部通过的 P0 Acceptance Criteria。

## AC-01：镜像可以独立构建

执行：

```bash
docker build -f Dockerfile.daemon-pi -t dars-daemon-pi .
```

必须：

- 构建成功；
- 镜像内存在 `dars`；
- 镜像内存在 `pi`；
- `dars --version` 可执行；
- `pi --version` 可执行。

---

## AC-02：容器可以自动拉起 daemon

提供必要环境变量与 volume 后，执行一次 `docker run` / `docker compose up` 即可。

必须：

- 不进入容器执行任何人工命令；
- 不执行 `dars login`；
- ENTRYPOINT 自动完成 Preflight；
- Preflight 通过后自动执行 `dars daemon start --foreground`；
- daemon 成为容器主业务进程。

---

## AC-03：Pi 最必要连通性检查成功

启动时必须验证：

- Pi CLI 存在；
- Pi CLI 可启动；
- 模型配置可解析；
- Provider Credential 可加载。

验收环境必须至少执行一次 `DARS_PI_PREFLIGHT=active`，确认 Pi 可以真正发起模型调用并得到有效响应。

如果 Pi 无法调用模型：

- 容器启动失败；
- daemon 不启动；
- DARS 不应出现该 Runtime 为 online。

---

## AC-04：可以正常连接远程 DARS

使用：

```text
DARS_SERVER_URL=<remote server>
DARS_WORKSPACE_ID=<workspace>
DARS_DAEMON_ID=<daemon>
DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token
```

简单环境变量模式可以用 `DARS_DAEMON_TOKEN=<ddt_...>`，生产示例优先 token file。

必须：

- 无需 `dars login`；
- 可以通过 Server Auth Preflight；
- daemon 可以成功建立与远程 DARS 的运行连接；
- heartbeat 正常。

错误 Token 时必须失败，并输出明确认证错误。

不可达 Server 时必须失败，并输出明确网络错误。

---

## AC-05：DARS 可以发现该 Pi Runtime

容器启动完成后，在 DARS Runtime 列表 / API 中必须能够看到新节点。

至少可以确认：

- daemon identity；
- device name；
- runtime name；
- provider / runtime 类型为 Pi；
- online / healthy 状态；
- 最近 heartbeat 正常更新。

节点应在 HEALTHCHECK `start_period` 覆盖的启动窗口内完成注册；basic 模式目标不超过 60 秒，active 模式可按 Provider 延迟放宽。

---

## AC-06：DARS 可以向该 Runtime 下发任务

在 DARS 创建一个明确路由到该 Runtime 的测试 Task。

必须：

1. Server 成功调度 Task；
2. daemon 成功 claim Task；
3. daemon 正确准备 workspace；
4. daemon 成功 spawn Pi；
5. Pi 使用预期 Provider / Model；
6. Pi 开始产生执行事件；
7. DARS 可以收到任务运行状态。

---

## AC-07：Pi 可以正确完成真实任务

P0 E2E Smoke Task 建议使用确定性任务：

```text
使用 shell 工具执行：printf DARS_PI_E2E_OK
将 stdout 原样作为最终答案，不要添加其他文字。
```

测试工作区必须是 empty-repos，智能体必须明确绑定目标 Pi Runtime/model 且不绑定 ToolBundle，避免 DARS 工具注入改变任务行为。通过条件：

- Pi Agent 实际启动；
- Pi 实际调用 shell/tool；
- 已持久化的 tool-result 包含 `DARS_PI_E2E_OK`；
- 经 Server TrimSpace/脱敏归一化后，Pi 最终输出精确等于 `DARS_PI_E2E_OK`；
- 任务执行进程正常退出。

该测试同时验证：

```text
Model
+ Agent Loop
+ Tool Execution
+ Workspace
+ JSON Event Stream
```

均可用。

---

## AC-08：任务结果可以正确提交到 DARS

AC-07 完成后，DARS Server 上必须能够看到：

- 数据库恰好有一条 terminal `completed` 记录；
- 最终持久化输出精确为 `DARS_PI_E2E_OK`；
- 任务关联到正确 daemon / runtime；
- 运行期间产生的状态消息能够被 DARS 接收；
- tool-use / tool-result 等当前 DARS 已支持的 Pi 事件能够正常上报；
- 若 Pi 返回 usage，则 usage 能沿现有链路记录；
- 不发生重复 completed / 重复 result 提交。

这一条是整个 `Dockerfile.daemon-pi` 最终 P0 验收门槛。

---

## AC-09：完整冷启动 E2E 一次通过

必须提供一个可以重复执行的自动化 Smoke Test，验证：

```text
build image
    ↓
start container
    ↓
Pi preflight passed
    ↓
DARS auth passed
    ↓
daemon started
    ↓
runtime registered
    ↓
server dispatch task
    ↓
daemon claim
    ↓
Pi execute
    ↓
result submitted
    ↓
Task completed
```

测试不得依赖人工点击或进入容器执行命令。它必须由 `DARS_RUN_REAL_PI_CONTAINER_SMOKE=1` 显式授权，使用隔离 PostgreSQL/Server、唯一 daemon ID 和临时 mode-0700 Pi config 工作副本；默认 `make check` 不得消费真实 Provider quota。

---

## AC-10：Config / State Volume 重启可复用

挂载已有 Pi config 与持久化 DARS state 后：

1. 第一次启动成功；
2. 停止容器；
3. 使用相同 volume 重建 / 重启容器；
4. Pi config 继续生效；
5. daemon identity 保持稳定；
6. Runtime 可以重新上线；
7. 不需要重新执行 login。

还必须使用相同 volume 与同一未过期 `ddt_` 完成第二个真实 task，证明优雅停止没有吊销凭据、Runtime identity 未重复、工作区状态可复用。

---

# 12. 失败场景验收

## F-01 Pi binary 缺失

预期：

```text
startup failed
runtime not registered
clear error log
```

## F-02 Model 配置错误

预期：Pi Preflight 失败，daemon 不启动。

## F-03 Provider credential 错误

active preflight 必须检测到模型调用失败。

## F-04 DARS Server 不可达

预期：DARS connectivity preflight 失败，不进入 Ready。

## F-05 Daemon Token 错误

预期：错误 prefix、过期、已撤销、Workspace/daemon 越界、owner 未解析，或权威 token file 不可读/为空时认证失败，不回退其他凭据，不注册 online Runtime。

## F-06 DARS 连接建立后中断

预期：daemon 按现有重连机制工作；Server 恢复后使用相同 identity/Token 重新上线，Runtime 状态最终反映 offline / online 变化，不产生新的重复 Runtime identity，并可完成后续 task。

## F-07 Pi Task 执行失败

预期：

- daemon 不崩溃；
- Task 被正确标记 failed / 对应错误状态；
- 错误信息被提交到 DARS；
- 后续新 Task 仍可执行。

自动化验收必须先制造一次 failed task，再下发一个成功 task；成功 task 的完成用于证明 Runtime 没有进入不可恢复状态。

---

# 13. 可观测性要求

启动日志至少应包含结构化阶段信息：

```text
[daemon-pi] validating configuration
[daemon-pi] pi preflight started
[daemon-pi] pi ready
[daemon-pi] dars connectivity preflight started
[daemon-pi] dars authenticated
[daemon-pi] starting daemon
[daemon-pi] runtime registered
[daemon-pi] ready
```

不得在日志中打印：

- `DARS_DAEMON_TOKEN`；
- `DARS_TOKEN`；
- Provider API Key；
- Pi `auth.json` 中的敏感凭据。

错误日志必须明确指出失败阶段，而不是统一输出“startup failed”。

---

# 14. 安全要求

1. 容器默认以非 root 用户运行；
2. 不把 daemon 长期 Token 注入 Task Runtime；
3. Pi task 只获得当前任务需要的 task-scoped credential；
4. Provider API Key 不写入镜像 layer；
5. Secret 仅通过运行时环境变量 / secret mount / config volume 注入；
6. 默认不挂载宿主机 Docker Socket；
7. 默认不挂载整个宿主机 HOME；
8. workspaces 与 credential 目录分离。

---

# 15. 第一阶段交付物

P0 实现至少包括：

```text
Dockerfile.daemon-pi

docker/daemon-pi-entrypoint.sh

docker-compose.daemon-pi.yml

Workspace-admin Daemon Token 预配/list/rotate/revoke API 与 CLI

`DARS_DAEMON_TOKEN_FILE` / `DARS_DAEMON_TOKEN` fail-closed resolver

Pi Preflight

DARS Server Preflight

Docker health/readiness logic

E2E smoke test

确定性容器测试

`server/internal/service/builtin_skills/dars-runtime-protocol/SKILL.md` 协议更新

145-route / 35-table contracts

使用文档与脱敏验收证据
```

---

# 16. Definition of Done

本需求不以“Docker image build 成功”为完成标准。

只有下面的黑盒链路可以稳定重复通过，才视为 Done：

```text
给定：
  DARS Server URL
  + Daemon Token
  + Pi Model / Provider Credential 或已有 Pi Config

执行：
  docker compose up

系统自动完成：
  Pi 自检
  → DARS 认证
  → daemon 启动
  → Runtime 注册
  → Runtime 被 DARS 发现
  → DARS 下发 Task
  → Pi 执行 Task
  → daemon 回传执行过程与结果
  → DARS Task = completed
```

最终验收断言：

```text
Runtime visible = true
Runtime online = true
Task dispatched = true
Task claimed = true
Pi invoked = true
Tool executed = true
Exactly one terminal completed row = true
Final output = DARS_PI_E2E_OK
Persisted tool-result contains DARS_PI_E2E_OK = true
Result persisted in DARS = true
Same-token restart task completed = true
Reconnect recovery = true
Failure-then-success recovery = true
```

验收证据只保留镜像 digest、DARS/Node/Pi 版本、非敏感 Runtime/Task/数据库断言和时间戳；不得包含 Daemon Token、Provider credential、Pi `auth.json` 或完整用户配置。

---

# 17. 实现与验收结果

2026-08-28 已使用隔离 PostgreSQL/Server、empty-repos Workspace 和 `/Users/logo/.pi/agent` 的 mode-0700 临时工作副本完成真实 Pi 容器黑盒验收。验收覆盖 active preflight、Runtime online/heartbeat、真实 tool-result、精确最终输出 `DARS_PI_E2E_OK`、同 Token/同 volumes 重建、Server 断开后重连、失败 task 后恢复以及唯一 terminal completion projection。

脱敏证据保存在 `openspec/changes/add-daemon-pi-container-runtime/evidence/real-pi-container-smoke.json`；证据中 `secrets_retained=false`，不包含 `ddt_`/`dpat_`/`dat_`、Provider credential、`auth.json`、API key、OAuth 值或私钥。

最终发布门禁 `DARS_RUN_DAEMON_PI_IMAGE_TEST=1 make check` 通过，包含目标 TypeScript typecheck/build/unit contracts、Fresh Lightweight DB migrations、Go 全量测试、daemon-pi 确定性镜像契约和 Playwright 浏览器 E2E。
