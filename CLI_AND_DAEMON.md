# DARS CLI 与 Daemon

本文仅说明 Lightweight 产品当前保留的 CLI 与本地 Daemon 能力。具体参数以本机安装版本的 `--help` 输出为准。

## 快速开始

```bash
dars setup self-host
dars login
dars daemon start
dars daemon status
```

使用多个环境时，通过 `--profile` 隔离配置、Daemon 状态和 Workspace 选择：

```bash
dars --profile dev login
dars --profile dev daemon start
```

## 当前命令

| 命令 | 用途 |
|---|---|
| `dars auth` | 查看或清理认证状态 |
| `dars login` | 使用邮箱验证码或现有 PAT 登录 |
| `dars config` | 管理 CLI 配置 |
| `dars setup` | 配置自部署 Lightweight Runtime |
| `dars workspace` | 管理当前 Workspace |
| `dars runtime` | 管理本地 Runtime 与 Profile |
| `dars daemon` | 启动、停止和检查本地 Daemon |
| `dars agent` | 管理 Agent |
| `dars skill` | 管理 Skill |
| `dars chat` | 与 Agent Direct Chat |
| `dars squad` | 管理仅包含 Agent 的 Squad |
| `dars issue` | 管理 Lightweight Run |
| `dars user` | 查看或更新当前用户 |
| `dars version` | 查看版本 |

查看任意命令的完整参数：

```bash
dars --help
dars daemon --help
dars issue create --help
```

## Daemon 职责

Daemon 运行在本地机器上，负责：

- 注册本地 Runtime；
- 发现可用的 Agent CLI；
- 从 Server 领取 Task；
- 准备隔离的工作目录与运行环境；
- 执行 Agent 并上报消息、用量和最终状态；
- 在断线、取消或进程异常后执行恢复和清理。

Daemon 不承载 Web、Server 或 PostgreSQL，也不提供 Cloud Runtime。

## Workspace Daemon Token

容器或无交互节点使用 Workspace 级 `ddt_` Daemon Token。只有 Workspace owner/admin 可以创建、轮换、列出元数据或吊销；明文只在创建/轮换响应中出现一次。默认有效期 90 天，显式过期时间最长 365 天。

```bash
dars daemon token create --daemon-id daemon-pi-prod --name "Pi production" > daemon-token.json
jq -r .token daemon-token.json > daemon-token.secret
chmod 600 daemon-token.secret

dars daemon token list
dars daemon token revoke <token-id>
```

同一 `daemon-id` 再次执行 `create` 会轮换凭据并把对应 Runtime 标为离线。正常 `dars daemon stop` 或容器重启不会吊销 Token；复用相同 Token、Daemon ID 和 DARS state volume 会更新原 Runtime identity。

## daemon-pi 容器

仓库提供 `Dockerfile.daemon-pi` 与 `docker-compose.daemon-pi.yml`。镜像固定 Node 22.19.0 与 Pi 0.84.2，以非 root 用户运行，并通过 `tini` 转发信号。下面的例子复用本机现有 Pi 配置 `/Users/logo/.pi/agent`：

```bash
export DARS_SERVER_URL=http://host.docker.internal:8080
export DARS_WORKSPACE_ID=<workspace-uuid>
export DARS_DAEMON_ID=daemon-pi-local
export DARS_PI_CONFIG_DIR=/Users/logo/.pi/agent
export DARS_DAEMON_TOKEN_FILE=/run/secrets/dars_daemon_token
export DARS_DAEMON_TOKEN_SECRET_FILE="$PWD/daemon-token.secret"
export DARS_UID="$(id -u)"
export DARS_GID="$(id -g)"
export DARS_PI_PREFLIGHT=active

docker compose -f docker-compose.daemon-pi.yml up --build -d
docker compose -f docker-compose.daemon-pi.yml ps
docker compose -f docker-compose.daemon-pi.yml logs -f daemon-pi
```

`basic` 只验证模型 catalog 与内建 Provider 的认证状态；extension/custom Provider 必须使用 `active`。`active` 会在空临时目录中关闭 session、tool、project context 和 stdin，发起一次真实最小模型调用。OAuth 配置应可写以便刷新；只读挂载会使用 `--no-refresh`，过期时启动失败。

凭据优先级为 `DARS_DAEMON_TOKEN_FILE` → `DARS_DAEMON_TOKEN` → 本地 profile。只要设置了 file 变量，文件缺失、不可读、为空或不是 `ddt_` 都会直接失败，不会回退。容器启动仅接受 file/env `ddt_`。

## 常用环境变量

| 变量 | 用途 |
|---|---|
| `DARS_SERVER_URL` | 覆盖 Server 地址 |
| `DARS_WORKSPACE_ID` | 指定当前 Workspace |
| `DARS_DAEMON_TOKEN_FILE` | 从文件读取 Workspace Daemon Token；设置后权威且 fail closed |
| `DARS_DAEMON_TOKEN` | 直接提供 `ddt_`；容器优先使用文件 |
| `DARS_PI_MODEL` | daemon-pi 的精确 `provider/model` |
| `DARS_PI_PREFLIGHT` | daemon-pi 的 `basic`、`active` 或显式 `disabled` |
| `DARS_DEBUG` | 输出完整错误信息 |

更多部署配置见 [SELF_HOSTING.md](SELF_HOSTING.md)，安装方式见 [CLI_INSTALL.md](CLI_INSTALL.md)。
