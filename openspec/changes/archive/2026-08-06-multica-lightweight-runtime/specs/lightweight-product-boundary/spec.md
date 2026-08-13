## Purpose

定义 Multica 轻量版作为单一产品的交付边界、保留体验、退出能力与构建范围，防止实现过程中恢复完整产品功能或引入兼容模式。

## ADDED Requirements

### Requirement: 单一轻量产品替换当前复制版本
仓库 SHALL 只交付一个 Lightweight 产品形态，不得提供 Full/Light 运行模式、功能开关或同进程兼容层。目标产品 SHALL 以 Web、Server 和 Local Daemon/CLI 构成可运行闭环。

#### Scenario: 启动目标产品
- **WHEN** 操作者按目标发行配置启动系统
- **THEN** 仅启动 Lightweight Web、Server 和 Local Daemon/CLI 所需组件
- **THEN** 配置中不存在切换回 Full 产品的模式

### Requirement: 保留最小产品表面
Web SHALL 提供邮箱登录、Workspace 选择与设置、只读成员列表、Daemon/Runtime、Runtime Profile、Skill、Agent、Direct Chat、Agent-only Squad、Lightweight Run 列表与详情。Run 详情 SHALL 将 Flat Comments 与 Task Runs 分区展示，并以 Issue 作为持久化根对象。

#### Scenario: 用户完成主导航闭环
- **WHEN** 已登录用户从目标 Web 导航依次访问 Runtime、Agent、Chat、Squad 和 Run
- **THEN** 每个保留入口均可访问且只依赖目标 API
- **THEN** Run 详情不依赖 Project Board、Issue Table 或混合 Timeline

### Requirement: 退出完整产品外围能力
目标产品 MUST 移除 Cloud Runtime、Attachment、Human Squad Member、Workspace Invitation、Agent Template/Builder、Runtime Self-update、Autopilot、Inbox、Channel、Slack、Lark/Feishu、Billing、Notification、External Webhook、Project/Dashboard/Complex Search、Issue Board/Table、Property、Label、Subscriber、Reaction、Calendar、Child Issue、Quick Action、VCS/PR 管理以及相关页面、导航、命令、内置 Skill、API 和后台服务。

#### Scenario: 访问退出能力
- **WHEN** 客户端访问任一退出页面深链、API 代表路径或 CLI 命令
- **THEN** 系统返回不可用或 404，且不会构造旧 Handler 或触发旧后台副作用

#### Scenario: 扫描目标产物
- **WHEN** 发布流程检查 Web route、Sidebar、全局搜索、Modal、API Client、CLI、内置 Skills、i18n、assets 和生产依赖
- **THEN** 不存在可恢复退出能力的用户入口或运行依赖

### Requirement: 目标构建图排除非交付应用
根级 build、typecheck、test、lint、`make check` 和 required CI SHALL 只覆盖 Web 与目标共享包，不得构建或验收 Desktop、Mobile 或 Docs。Desktop 专属源码在本变更中 SHALL 保持未改造状态，且不得以兼容 Desktop 为由保留旧 API。

#### Scenario: 执行根级前端命令
- **WHEN** 在 Fresh Checkout 执行根级 build、typecheck、test 和 lint
- **THEN** 只运行 Web 与目标共享包任务
- **THEN** Desktop、Mobile 和 Docs 均不在执行图中

#### Scenario: 共享包发生变更
- **WHEN** Core、View 或 UI 共享代码发生修改并触发 required CI
- **THEN** CI 验证目标 Web 工作集
- **THEN** Desktop 构建失败不构成本变更的发布门禁

### Requirement: 不提供旧契约兼容
Server、Web 与 Daemon/CLI MUST 同批切换到 Lightweight 契约，不得保留旧 API alias、旧 Desktop 适配器、旧数据库升级器或原完整产品数据库运行模式。

#### Scenario: 旧客户端调用
- **WHEN** 旧 Desktop 或旧 CLI 调用未列入 Lightweight manifest 的路径或字段
- **THEN** 请求被拒绝而不是被兼容层转换

#### Scenario: 新客户端运行
- **WHEN** 目标 Web 与 Daemon/CLI 对同一 Server 运行
- **THEN** 三端使用同一冻结 manifest 和 DTO 契约完成 P0 流程

### Requirement: 范围变更必须显式立项
本变更未列入的能力 MUST NOT 在实现中隐式恢复；新增产品范围、兼容要求或量化阈值调整 SHALL 通过独立 OpenSpec change 审批。

#### Scenario: 实现需要恢复已退出能力
- **WHEN** 某实现任务提出重新启用 Cloud、Attachment、Invitation、Desktop 兼容或其他退出能力
- **THEN** 当前变更停止吸收该范围，并要求建立独立 change
