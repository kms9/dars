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

## 常用环境变量

| 变量 | 用途 |
|---|---|
| `DARS_SERVER_URL` | 覆盖 Server 地址 |
| `DARS_WORKSPACE_ID` | 指定当前 Workspace |
| `DARS_DEBUG` | 输出完整错误信息 |

更多部署配置见 [SELF_HOSTING.md](SELF_HOSTING.md)，安装方式见 [CLI_INSTALL.md](CLI_INSTALL.md)。
