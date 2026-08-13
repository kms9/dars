## MODIFIED Requirements

### Requirement: 保留最小产品表面
Web SHALL 提供邮箱登录、Workspace 选择与设置、只读成员列表、Daemon/Runtime、Runtime Profile、Skill、Agent、Direct Chat、Agent-only Squad、Lightweight Run 列表与详情。Run 详情 SHALL 将 Flat Comments 与 Task Runs 分区展示，并以 Issue 作为持久化根对象。

Skill 表面 SHALL 包含 List/Create/Detail、文件管理、Agent Binding，以及三种创建/导入入口：手动创建、Import from URL（含 archive 上传）、Copy from runtime。该扩展不得恢复 Skill Labels、Agent Template/Builder 或其他已退出外围能力。

#### Scenario: 用户完成主导航闭环
- **WHEN** 已登录用户从目标 Web 导航依次访问 Runtime、Agent、Chat、Squad 和 Run
- **THEN** 每个保留入口均可访问且只依赖目标 API
- **THEN** Run 详情不依赖 Project Board、Issue Table 或混合 Timeline

#### Scenario: 用户完成 Skills 导入与管理
- **WHEN** 已登录用户打开 Skills 页并使用 Manual、URL/Archive 或 Runtime 导入创建 Skill，再编辑文件并绑定到 Agent
- **THEN** 流程只依赖目标 Skill/Runtime APIs 完成
- **THEN** 不出现 Labels、Template Builder 或 Desktop 专属入口
