# DARS Lightweight 自部署高级配置

以 `.env.example` 为权威变量清单。本文只说明生产部署中容易配置错误的部分。

## 数据库身份

```dotenv
POSTGRES_DB=dars_lightweight
EXPECTED_DATABASE_NAME=dars_lightweight
DARS_EDITION=lightweight
DATABASE_URL=postgres://dars:<password>@postgres:5432/dars_lightweight?sslmode=disable
```

Server 会校验数据库名称、Edition 与 Schema 标记。不要连接旧完整产品的 `dars` 数据库；Lightweight 不回放旧迁移链，也不提供原地升级。

连接池可通过以下变量调整：

- `DATABASE_MAX_CONNS`
- `DATABASE_MIN_CONNS`
- `DATABASE_MAX_IDLE_CONNS`
- `DATABASE_MAX_CONN_LIFETIME`

## Secret

生产环境至少要设置：

```bash
openssl rand -hex 32
openssl rand -base64 32
```

分别用于 `JWT_SECRET` 与 `DARS_AGENT_SECRET_KEY`。后者用于加密 Agent 环境变量与配置，必须保存到独立 Secret 管理系统，并随数据库备份一起恢复。轮换时通过 `DARS_AGENT_SECRET_KEY_ID` 标识当前密钥。

## 邮箱认证

设置 `SMTP_HOST` 时优先使用 SMTP，否则使用 Resend。

Resend：

```dotenv
RESEND_API_KEY=
RESEND_FROM_EMAIL=noreply@example.com
```

SMTP：

```dotenv
SMTP_HOST=
SMTP_PORT=587
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM_EMAIL=
SMTP_TLS=starttls
SMTP_TLS_INSECURE=false
SMTP_EHLO_NAME=
```

`DARS_DEV_VERIFICATION_CODE` 只用于非生产测试，不应在公网环境启用。

## 注册与 Workspace 策略

| 变量 | 用途 |
|---|---|
| `ALLOW_SIGNUP` | 是否允许创建新账号 |
| `ALLOWED_EMAIL_DOMAINS` | 允许注册的邮箱域名列表 |
| `ALLOWED_EMAILS` | 允许注册的完整邮箱列表 |
| `DISABLE_WORKSPACE_CREATION` | 是否禁止用户创建新 Workspace |
| `AUTH_TOKEN_TTL` | Token 有效期 |

Lightweight 不提供 Workspace Invitation 或成员变更接口。关闭注册或 Workspace 创建前，应先建立所需账号与 Workspace。

## Redis 与限流

`REDIS_URL` 可启用跨实例 Realtime Relay、Token Cache 和公开认证限流。认证限流由以下变量控制：

- `RATE_LIMIT_AUTH`
- `RATE_LIMIT_AUTH_VERIFY`
- `RATE_LIMIT_TRUSTED_PROXIES`

单实例可以不配置 Redis。

## URL、Cookie 与代理

公网部署常用变量：

```dotenv
FRONTEND_ORIGIN=https://dars.example.com
DARS_APP_URL=https://dars.example.com
DARS_PUBLIC_URL=https://api.dars.example.com
CORS_ALLOWED_ORIGINS=https://dars.example.com
COOKIE_DOMAIN=
DARS_TRUSTED_PROXIES=
```

反向代理必须支持 WebSocket，并将 `/ws` 转发到 Server。只有来自 `DARS_TRUSTED_PROXIES` 的转发头会被信任。

## Metrics 与日志

- `METRICS_ADDR`：启用独立 Metrics Listener；
- `LOG_LEVEL`：设置日志级别；
- `/health`：进程存活；
- `/readyz`：数据库等依赖是否就绪。

Lightweight 不启动旧 Product Analytics、Usage Rollup、Notification、Autopilot、Webhook 或 Channel Worker。

## 升级与备份

升级前备份：

1. `dars_lightweight` PostgreSQL 数据库；
2. `DARS_AGENT_SECRET_KEY` 及其 ID；
3. `.env` 中的部署 Secret。

```bash
docker compose -f docker-compose.selfhost.yml pull
docker compose -f docker-compose.selfhost.yml up -d
curl http://localhost:8080/readyz
```

不要把旧完整产品数据库作为回滚目标。回滚应使用同一 Lightweight 数据库的备份、匹配版本的镜像与对应 Secret。
