## MODIFIED Requirements

### Requirement: Route、Worker 和 schema manifests 是硬门禁
发布 SHALL 要求 Router dump 与 checked-in 145 条 route manifest 精确相等、运行 Worker 与既有批准清单精确相等、`pg_catalog` 与 checked-in 35 表 schema manifest 精确相等。页面出现、单元测试通过、自描述 health 或只检查总数不得替代集合级证据。

#### Scenario: 任一清单漂移
- **WHEN** Router 多/少/替换任一路由、运行中出现额外 Worker，或数据库应用表集合不是批准 35 表
- **THEN** 发布门禁失败并输出集合 diff

#### Scenario: daemon-pi 路由计入门禁
- **WHEN** 发布流程 dump Router
- **THEN** 结果在当前 142 条活跃基线上包含 3 条工作区 Daemon Token 管理路由
- **THEN** 总数精确为 145，且未批准的 Desktop alias 与其他业务路径不存在

## ADDED Requirements

### Requirement: daemon-pi 容器闭环进入 Runtime 发布门禁
Runtime 发布 SHALL 构建 daemon-pi 候选镜像并在显式启用真实 Provider 的隔离环境中完成 container-to-Server-to-Pi-to-Server 黑盒 smoke。证据 MUST 覆盖镜像内容、非 root/PID 1、Pi active preflight、Daemon Token 预配/auth、运行时注册/heartbeat、task dispatch/claim、Pi tool execution、tool-result、消息/usage 上报、唯一 completed 终态、结果持久化、连接中断恢复、失败后继续服务和同 volume 重启；静态 Dockerfile 检查、mock Pi、只到运行时 online 或只到进程退出均不得替代该门禁。

#### Scenario: 发布候选完成真实闭环
- **WHEN** 候选版本在隔离 PostgreSQL/Server 与真实 Provider credential 上运行 required daemon-pi smoke
- **THEN** 全部容器与任务断言通过，证据记录候选镜像 digest、版本和非敏感 Runtime/Task/数据库结果

#### Scenario: 未授权或缺少真实 Provider
- **WHEN** 默认工程检查未显式启用真实 Pi Provider 调用，或环境缺少 credential
- **THEN** 确定性单元/容器结构检查可运行，但真实闭环门禁标记为未运行而不是通过，且不访问用户安装的 Pi 或消耗额度

#### Scenario: 任一闭环阶段缺失
- **WHEN** build、active preflight、registration、dispatch、tool execution、result persistence 或重启复用任一阶段失败或无证据
- **THEN** daemon-pi 发布门禁失败，并保留已通过与未通过阶段的边界
