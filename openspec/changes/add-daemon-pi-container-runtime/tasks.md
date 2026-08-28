## 1. Daemon Token 数据与专用 API

- [x] 1.1 为 `daemon_token.user_id/name/token_prefix` 编写双向 migration 与保守回填验证，不新增表、foreign key 或索引，并更新 sqlc 生成代码和 35 表 column contracts
- [x] 1.2 先添加三个 Workspace-admin Daemon Token API 的路由、schema、权限、明文一次可见、90 天默认/365 天上限和 rotate/revoke-offline 测试
- [x] 1.3 实现 `GET/POST /api/workspaces/{workspaceId}/daemon-tokens` 与 `DELETE /api/workspaces/{workspaceId}/daemon-tokens/{tokenId}`，保持 `/api/tokens` PAT 接口兼容
- [x] 1.4 将当前 142 条路由 contract 更新为精确 145 条，并修正 route/schema metadata 中的历史 126/27 基线
- [x] 1.5 添加 `dars daemon token create/list/revoke` 命令、受控 client tests 与 Workspace 管理员操作说明

## 2. Register 与 Token 生命周期

- [x] 2.1 先添加 Register 使用 Human 与匹配、越界、过期、owner 未解析 `ddt_` 的 middleware/handler tests
- [x] 2.2 实现 Register 的 Human-or-Daemon auth 分流：Human 路径轮换并返回 `ddt_`，匹配的预配 `ddt_` 路径保留原 token 且不返回明文
- [x] 2.3 实现 Workspace 机器凭据所有权语义：签发人离开不失效，当前管理员可 rotate/revoke，rotation 更新 runtime owner seed
- [x] 2.4 将 graceful Deregister 改为仅标记 runtime offline；仅 expiry、Workspace 删除、显式 rotate/revoke 使 Token 失效，并验证相同 Token/volume 可重启

## 3. 守护进程非交互认证与环境隔离

- [x] 3.1 添加共享 resolver tests，覆盖 `DARS_DAEMON_TOKEN_FILE` > `DARS_DAEMON_TOKEN` > profile、file 设置即权威且错误时 fail closed、容器仅接受 `ddt_`
- [x] 3.2 实现 resolver、`DARS_WORKSPACE_ID` 环境优先级，以及后台/前台一致验证；`Client.SetToken(ddt_)` 必须清除陈旧 Human pairing token
- [x] 3.3 实现不注册 Runtime 的 auth preflight 命令，通过 `/api/daemon/workspaces` 精确验证 Token 的 Workspace scope
- [x] 3.4 验证 Pi task child env 移除全部继承的 `DARS_*` 后仅注入 task-scoped `dat_`，并让 ACP model discovery 使用相同净化基础环境且保留 Provider credential

## 4. Pi Preflight、Readiness 与容器发行物

- [x] 4.1 为 Pi basic/active/disabled、内建/自定义 Provider、只读 OAuth、closed stdin 与错误阶段添加 entrypoint 测试夹具
- [x] 4.2 实现精确模型解析；basic 使用 `pi auth check --json`，只读配置附加 `--no-refresh`，自定义 Provider 要求 active；active 在临时空目录中禁用 session/tools/context 并验证 sentinel
- [x] 4.3 为本地 health response 添加 runtime ID→provider 的非敏感映射并保持 `workspaces[].runtimes []string` 兼容
- [x] 4.4 实现 `dars daemon container-health`，严格校验 `running`、目标 Workspace 与已注册 Pi Runtime
- [x] 4.5 实现 entrypoint 的配置/权限检查、分阶段无 Secret 日志与最终 `exec dars daemon start --foreground`
- [x] 4.6 实现精确 Go、Node `>=22.19.0` patch、Pi `0.84.2`、非 root UID/GID、执行依赖、`tini`、OCI labels 和带 `start_period` 的 `Dockerfile.daemon-pi`
- [x] 4.7 实现 `docker-compose.daemon-pi.yml`，支持 token file/env、本机或远程 Server，并分离可写 Pi config、DARS state 与 task workspace 三个 volume

## 5. 确定性验证、协议文档与发布门禁

- [x] 5.1 添加不访问用户 Pi 的镜像内容、版本、默认用户、PID 1、Secret absence、错误配置和 health 状态容器测试
- [x] 5.2 覆盖错误 Pi binary/model/credential、自定义 Provider basic、错误/越界/过期 Daemon Token、不可达 Server、rotate/revoke 与非 healthy 状态
- [x] 5.3 更新 `server/internal/service/builtin_skills/dars-runtime-protocol/SKILL.md`、运维/环境变量文档与 `/Users/logo/.pi/agent` 挂载、Token 预配轮换、私有 CA、UID/GID 示例
- [x] 5.4 更新 `docs/prd/Dockerfile.daemon-pi-需求文档.md` 与本 change artifacts，记录最终协议、风险、验收证据和 145 路由/35 表基线
- [x] 5.5 运行 OpenSpec strict validation、Go/TypeScript targeted tests、Docker deterministic tests 和 `make check`，修复全部回归

## 6. 真实 Pi 黑盒验收

- [x] 6.1 实现由 `DARS_RUN_REAL_PI_CONTAINER_SMOKE=1` 显式启用、使用隔离 PostgreSQL/Server 与 empty-repos Workspace 的 smoke harness，并只保留脱敏 JSON evidence
- [x] 6.2 使用 `/Users/logo/.pi/agent` 的 mode-0700 临时工作副本、active preflight 和真实 Provider 启动 daemon-pi，验证目标 Pi Runtime 注册、online 与 heartbeat
- [x] 6.3 创建不绑定 ToolBundle、明确绑定 Pi Runtime/model 的智能体并下发确定性 shell task，验证 tool-result 包含 sentinel、唯一 completed terminal row 与精确最终输出 `DARS_PI_E2E_OK`
- [x] 6.4 使用相同 volumes 重建容器并运行第二个成功 task；验证相同 `ddt_` 无需登录重新注册，以及 Server 断开重连不产生重复 Runtime identity
- [x] 6.5 验证一次 Pi task 失败后容器/daemon 不崩溃且下一 task 成功，覆盖 F-06/F-07
- [x] 6.6 汇总镜像 digest、DARS/Node/Pi 版本、runtime/task/database 非敏感证据，并逐项对照 PRD AC/F/Definition of Done 完成审计
