# DARS Lightweight 自部署

当前自部署交付包含 Web、Go Server、PostgreSQL，以及运行在用户机器上的 CLI/Daemon。只支持新的 `dars_lightweight` 数据库，不支持将旧完整产品数据库原地升级。

## 前置条件

- Docker 与 Docker Compose；
- 安装 DARS CLI；
- 至少安装一种受支持的 Agent CLI。

## 启动服务

使用已发布镜像：

```bash
make selfhost
```

首次执行会从 `.env.example` 创建 `.env`，并生成随机的 `JWT_SECRET`、`POSTGRES_PASSWORD` 与 `DARS_AGENT_SECRET_KEY`。

若镜像尚未发布，可从当前源码构建：

```bash
make selfhost-build
```

默认地址：

- Web：`http://localhost:3000`
- Server：`http://localhost:8080`

## 配置 CLI 与 Daemon

```bash
dars setup self-host
dars daemon status
```

`setup self-host` 会配置 Server/Web 地址、完成登录并启动本地 Daemon。Daemon 自动发现当前机器上可用的 Agent CLI，并把本机注册为 Local Runtime。

## 邮箱登录

生产环境应配置一种邮件后端：

- Resend：`RESEND_API_KEY`、`RESEND_FROM_EMAIL`
- SMTP：`SMTP_HOST` 及对应的 `SMTP_*` 变量

未配置邮件后端时，验证码会写入 Server 日志。`DARS_DEV_VERIFICATION_CODE` 仅供非生产环境测试，生产环境会忽略该固定验证码。

## 公网部署

Compose 仅将端口绑定到 `127.0.0.1`。公网部署应通过 Caddy、nginx 或同类反向代理终止 TLS：

- 将 Web 请求转发到 `127.0.0.1:3000`；
- 将 API 与 `/ws` 转发到 `127.0.0.1:8080`；
- 设置正确的 `FRONTEND_ORIGIN`、`DARS_APP_URL`、`DARS_PUBLIC_URL` 与 `CORS_ALLOWED_ORIGINS`；
- 仅在可信代理后配置 `DARS_TRUSTED_PROXIES`。

不要直接把 Compose 端口改为 `0.0.0.0`。

## 常用操作

```bash
# 查看日志
docker compose -f docker-compose.selfhost.yml logs -f backend frontend

# 检查状态
curl http://localhost:8080/health
curl http://localhost:8080/readyz
dars daemon status

# 停止服务
dars daemon stop
make selfhost-stop
```

修改 `.env` 后重新执行 `make selfhost`。需要保留 `DARS_AGENT_SECRET_KEY`；丢失该密钥会导致已加密的 Agent 配置无法解密。

完整变量说明见 [SELF_HOSTING_ADVANCED.md](SELF_HOSTING_ADVANCED.md)，CLI 说明见 [CLI_AND_DAEMON.md](CLI_AND_DAEMON.md)。
