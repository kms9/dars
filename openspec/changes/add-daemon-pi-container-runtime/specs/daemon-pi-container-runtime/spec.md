## Purpose

定义一个可重复构建、无人值守启动并由 DARS 调度的容器化 Pi Runtime Node，以及证明任务从下发到结果持久化全部成功的黑盒验收契约。

## ADDED Requirements

### Requirement: daemon-pi 镜像必须可重复构建且不包含运行时 Secret
仓库 SHALL 提供独立的 daemon-pi 镜像定义。镜像 MUST 包含可执行的 `dars` 与固定版本的 `pi`、兼容的 Node.js、bash、git、ripgrep、CA certificates、OpenSSH client 和 PID 1 信号转发机制，默认 MUST 以非 root 用户运行。构建上下文、镜像层和默认配置 MUST NOT 包含 DARS、Provider 或 Pi Secret。

#### Scenario: 从仓库构建镜像
- **WHEN** 操作者从干净 checkout 执行文档规定的 `docker build -f Dockerfile.daemon-pi ...`
- **THEN** 构建成功，镜像内 `dars --version` 与 `pi --version` 均成功，且 Pi 版本等于构建时解析的固定版本

#### Scenario: 检查运行身份和镜像 Secret
- **WHEN** 验收检查镜像默认用户、history、environment 和文件系统
- **THEN** 默认用户不是 root，且未发现构建机的 DARS Token、Provider credential 或 Pi auth 内容

### Requirement: 容器配置必须支持无人值守认证和原生 Pi 配置
节点 SHALL 从运行时环境和挂载状态解析 DARS Server、工作区、稳定守护进程 identity、Daemon credential、Pi 模型和 Pi credential。`DARS_DAEMON_TOKEN_FILE` 与 `DARS_DAEMON_TOKEN` MUST 只接受 DARS 签发且绑定目标工作区与守护进程 identity 的运行期 `ddt_`；它们 MUST NOT 被解释为 Human PAT/JWT 或 task 级 `dat_`。解析优先级 MUST 为 token file、token environment、既有 profile config；token file 一旦设置就是唯一权威，缺失、不可读、空或前缀错误时 MUST fail closed 而不得回退。容器冷启动 MUST 要求 file/environment `ddt_`，profile PAT/JWT 回退只保留给既有本地守护进程流程。节点 SHALL 原样支持将宿主机 Pi 原生 config 目录（例如 `/Users/logo/.pi/agent`）挂载为 `PI_CODING_AGENT_DIR`，不得复制 Secret 到镜像。

#### Scenario: 使用 Daemon Token 和现有 Pi 配置冷启动
- **WHEN** 操作者提供 Server URL、工作区 ID、稳定 Daemon ID、匹配的 `ddt_` token，并把 `/Users/logo/.pi/agent` 挂载为 Pi config
- **THEN** 容器无需交互登录即可解析 DARS 与 Pi 配置并继续 preflight

#### Scenario: credential 类型或 scope 错误
- **WHEN** 权威 token input 是 `dpat_`、`dat_`、过期 `ddt_`，绑定到其他工作区/daemon，或设置的 token file 不可用
- **THEN** 启动在注册前 fail closed，并输出不含 token 内容的类型或 scope 错误

### Requirement: 启动必须按阶段 fail closed
容器 MUST 依次执行 configuration validation、Pi basic/active preflight、DARS daemon-token/auth preflight 和 daemon foreground start。任一前置阶段失败时 MUST 以非零状态退出，MUST NOT 启动 daemon claim loop，且 MUST NOT 留下 online Runtime。成功路径 MUST 以 `exec` 语义启动 foreground daemon，使 SIGTERM/SIGINT 到达 daemon 并触发其既有优雅退出逻辑。

#### Scenario: 前置条件失败
- **WHEN** Server URL 无效、必要目录不可写、Pi 不可执行、模型或 credential 不可解析，或 DARS enrollment 失败
- **THEN** 容器在对应阶段退出非零，daemon 不进入 running，日志给出可诊断且无 Secret 的阶段错误

#### Scenario: 前置条件全部成功
- **WHEN** configuration、Pi 和 DARS preflight 全部成功
- **THEN** entrypoint 用 foreground 模式启动 daemon，且容器停止信号可让 daemon 注销 Runtime 并退出

### Requirement: Pi preflight 必须验证实际选中的模型与认证
`basic` preflight SHALL 验证 Pi executable/version、Pi config 加载以及最终 Provider/Model 在完整 Pi model runtime 中唯一解析。对于 Pi 内置 Provider，basic MUST 再要求 `pi auth check --json` 为 ready 且不得请求或输出 credential；对于 extension/custom Provider，basic MUST 以明确诊断要求选择 `active`，不得把不加载 extension catalog 的 auth-check 失败误报为 credential 错误。只读 config MUST 使用 no-refresh auth check；需要 OAuth refresh 时 config MUST 可写。`active` preflight SHALL 在 basic 模型解析基础上，从隔离的临时工作目录关闭 stdin，以无 session、无工具和无项目 context 的方式执行一次真实最小模型调用，并要求 JSON event stream 正常结束且产生预期探针结果。`disabled` 仅可由操作者显式选择，且健康状态和日志 MUST 标明 Pi 未被验证。

#### Scenario: 使用挂载配置进行 basic preflight
- **WHEN** 挂载的 Pi config 定义默认 Provider/Model 且认证状态 ready
- **THEN** basic preflight 解析并记录非敏感的 Provider/Model 标识后成功，且不发起模型生成请求

#### Scenario: active preflight 验证真实连通性
- **WHEN** 选择 active 且 Provider、网络、credential 和模型均有效
- **THEN** Pi 完成固定低 token 探针并返回预期结果，证明 Agent Loop 与 JSON stream 可启动

#### Scenario: active preflight 的 Provider 调用失败
- **WHEN** DNS、endpoint、credential、quota 或模型可用性导致真实调用失败
- **THEN** 容器启动失败且 Runtime 不被报告为 online

#### Scenario: custom Provider 不能做离线认证检查
- **WHEN** 完整 Pi catalog 能解析 extension/custom Provider，但内置 `pi auth check` 不认识该 Provider
- **THEN** basic 返回要求 active 的明确结果，active 仍可通过真实调用验证该 Provider

### Requirement: Docker healthy 必须等价于 Runtime 可调度
Docker health/readiness SHALL 查询 daemon 的本地健康接口并且仅在状态为 `running`、目标工作区已存在且其中包含 provider 为 Pi 的运行时 ID 时返回成功。健康响应的现有 `workspaces[].runtimes` 字符串数组 MUST 保持兼容，provider 信息 SHALL 使用加性字段表达。Compose healthcheck MUST 配置足以覆盖 active preflight 与首次注册的 `start_period`。进程存在、端口可连接、Pi binary 存在或 daemon 状态为 `starting` 均 MUST NOT 单独产生 healthy。

#### Scenario: daemon 尚未完成注册
- **WHEN** daemon health endpoint 可达但状态为 `starting` 或目标 Workspace 尚无 Pi Runtime ID
- **THEN** Docker healthcheck 返回失败

#### Scenario: Pi Runtime 已注册并可 claim
- **WHEN** daemon 完成 enrollment、注册与初始 heartbeat，health 返回 `running` 且目标 Workspace 包含 Pi Runtime
- **THEN** Docker healthcheck 返回成功并保持与 daemon readiness 一致

### Requirement: Pi config、DARS state 与任务 workspace 必须分卷持久化
运行契约 MUST 分离 Pi config、DARS 守护进程 state 和 task workspaces。使用同一 DARS state volume 重启时，daemon identity MUST 保持稳定；使用同一 Pi config mount 时，Provider/Model/credential 与扩展配置 MUST 继续生效；DARS 管理的 Pi session 文件 MUST 位于持久化 DARS state 下。镜像/Compose SHALL 支持用宿主机 UID/GID 构建或运行非 root 用户，并在 Pi config 不可读、OAuth config 不可写或 state/workspace 不可写时报告 UID 与路径。默认配置 MUST NOT 挂载宿主机整个 HOME 或 Docker socket。

#### Scenario: 使用相同 volume 重建容器
- **WHEN** 第一次启动和任务完成后删除容器，再用相同 Pi config、DARS state 与 workspace volumes 创建新容器
- **THEN** daemon identity 不变、Pi 配置继续生效、Runtime 重新上线且不要求人工 login

### Requirement: DARS 必须完成 Pi 任务和结果回传闭环
验收 SHALL 使用真实 PostgreSQL、独立 DARS Server、daemon-pi 容器和真实 Pi Provider，在 repos 为空且智能体未绑定 ToolBundle 的隔离工作区创建明确路由到该 Pi 运行时的 task。task MUST 被唯一 claim，Pi MUST 实际执行 shell 工具得到 `DARS_PI_E2E_OK`，守护进程 MUST 上报至少一条包含该值的 tool-result、运行事件、usage（若 Pi 提供）和唯一终态，Server MUST 将 task 持久化为 `completed` 且按完成投影的同一 TrimSpace/redaction 规则得到精确最终输出 `DARS_PI_E2E_OK`。

#### Scenario: 冷启动后执行确定性任务
- **WHEN** smoke harness 构建镜像、启动容器、等待 Pi Runtime healthy，并下发规定的 shell 工具任务
- **THEN** Runtime 可见且 online，任务被目标 Runtime claim，Pi tool call 实际发生，最终结果和数据库状态均为 `DARS_PI_E2E_OK`/`completed`

#### Scenario: Pi 任务执行失败后继续服务
- **WHEN** 一个 Pi 任务因运行期错误进入 failed，随后下发一个有效任务
- **THEN** 首个任务持久化无 Secret 的失败信息，daemon 不崩溃，后续有效任务仍可完成且无重复终态提交

#### Scenario: Server 连接中断后恢复
- **WHEN** 已注册容器与 Server 的连接中断后恢复
- **THEN** 守护进程使用同一 identity 与仍有效的 `ddt_` 重连，运行时 online/offline 最终收敛且不产生重复运行时

#### Scenario: 同 volume 重启后再次执行
- **WHEN** 容器优雅停止后使用同一 volumes 与同一有效 `ddt_` 重建并下发第二个确定性 task
- **THEN** 第一个停止过程只把运行时置 offline、不销毁 token，第二次注册和 task 再次完整成功

### Requirement: 启动与验收证据必须可诊断且不泄密
启动日志 SHALL 标识 configuration、Pi basic/active、DARS daemon-token auth、daemon start、runtime registration 和 ready 阶段。自动化 smoke 结果 SHALL 保存镜像标识、DARS/Pi 版本、时间、Runtime/Task 非敏感 ID、阶段结果和最终断言；日志与证据 MUST 对 DARS Token、Provider credential、Pi auth 内容和任务级 `DARS_TOKEN` 做完全隐藏而非仅部分掩码。

#### Scenario: 收集成功证据
- **WHEN** 黑盒 smoke 成功
- **THEN** 证据足以复查每一阶段和最终数据库结果，但不能恢复任何 credential

#### Scenario: 收集失败证据
- **WHEN** 任一启动或任务阶段失败
- **THEN** 报告精确标明失败阶段和非敏感原因，且未运行的后续断言保持未通过而非被推断成功
