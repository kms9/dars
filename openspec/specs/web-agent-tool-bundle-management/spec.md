# web-agent-tool-bundle-management Specification

## Purpose
定义目标 Web 中为不同 Agent 选择不同 MCP/tools、发布不可变 ToolBundle、恢复当前选择和解释 Task pin 生命周期的管理体验。
## Requirements
### Requirement: Agent Capabilities 提供 Tools 子视图
目标 Web SHALL 在 Agent detail 的 `Capabilities` 下增加 `Tools` 子视图，而不是恢复 raw `view=mcp` 或 `agent.mcp_config` 编辑器。该子视图 SHALL 仅对 Workspace Owner/Admin 显示和允许写入，并从 Server Tool Source catalog 与 Agent current Bundle API 加载状态。

页面 SHALL 按 Source 分组显示 ready/enabled Tools，允许选择整个 Source 当前 revision 或单个 Tool，显示当前 Bundle ID、tool 数量、Source revision、disabled/revoked/update-available warning，并提供发布与清除操作。

#### Scenario: 不同 Agent 选择不同工具
- **WHEN** Admin 为 Agent A 选择 tools A/B，为 Agent B 选择 tools B/C 并分别发布
- **THEN** 两个 Agent head 指向独立不可变 Bundle，后续新 Task 只获得各自固定 Bundle 的 tools

#### Scenario: 页面刷新恢复选择
- **WHEN** Admin 发布选择后刷新 Agent Tools 子视图
- **THEN** 当前 Bundle ID 和选中 items 从 Agent current Bundle API 恢复，不依赖浏览器本地持久化

### Requirement: 发布操作遵守不可变 Bundle 语义
Web SHALL 发送完整 normalized Definition selection，等待 Server publication 成功后再更新界面。语义变化成功时 SHALL 显示新 Bundle ID；无变化保存 SHALL 保持当前 ID；恢复历史选择仍 SHALL 显示新 ID。失败时当前 selection/head MUST 保持已确认状态，页面显示稳定脱敏错误。

Source 新 revision MUST NOT 被自动选入或静默发布。页面 SHALL 显示 `update available`，只有 Admin 主动采用新 revision 并发布后，新 Task 才使用新 Bundle。已创建 Task 继续使用其旧 pin。

#### Scenario: Agent 更改选择
- **WHEN** Admin 从当前选择增加、删除 Tool 或采用 Source 新 revision并发布
- **THEN** Server 返回新 Bundle ID，页面切换到该 head，并说明现有 Task 不受影响、新 Task 使用新 Bundle

#### Scenario: Provider 不兼容
- **WHEN** Agent 当前 Runtime Provider 不支持 Gateway Remote HTTP MCP
- **THEN** 页面阻止或清晰报告 `provider_mcp_unsupported`，不得把 Agent 表示为已经获得 tools

### Requirement: Agent Tools 为每个选择项配置调用名称
Agent `Capabilities > Tools` SHALL 为每个选中的 Tool 显示只读 canonical public name，并提供 Agent exported name 输入。首次选择 SHALL 以 canonical public name 作为默认值；页面 SHALL 提供恢复 canonical name 的操作，并在发布前显示重复、空白、超长或非法字符错误。

页面刷新 SHALL 从 current Bundle 恢复 exported names，而不是根据当前 Source catalog 重新生成。页面 SHALL 明确说明该名称是 Agent 通过 MCP `tools/list`/`tools/call` 使用的身份，而 Source catalog identity 和 upstream operation 不会被修改。

#### Scenario: 配置 Agent 调用别名
- **WHEN** Admin 选择 `kratos-demo-http.User_GetUser` 并把调用名称改为 `skill.kratos-user.get_user`
- **THEN** 页面发布包含 Definition ID 与 exported name 的完整 selection，并在成功后展示新 Bundle ID、exported name 与只读 canonical name

#### Scenario: 刷新后恢复别名
- **WHEN** Admin 刷新已经发布自定义 exported name 的 Agent Tools 页面
- **THEN** 输入框从 Agent current Bundle 恢复原值，未把它覆盖为 Source 当前 canonical name

#### Scenario: 页面阻止名称冲突
- **WHEN** 两个选中 Tool 使用相同 exported name 或任一名称不符合 MCP-safe 规则
- **THEN** Publish 操作不可用并显示逐项或汇总错误，Server head 不改变

### Requirement: 清除和撤权在界面上语义分离
`Clear current tools` SHALL 只清除 Agent head，使后续新 Task 不再获得 Gateway Bundle；它 MUST NOT 被描述为撤销既有 Task。Bundle revoke 与 Source disable SHALL 作为紧急撤权操作清晰区分，并在确认后立即阻止受影响 active Task 的后续 discovery/call。

#### Scenario: 清除 Agent current Bundle
- **WHEN** Admin 确认清除 Agent 当前 tools
- **THEN** 页面显示 Agent 没有 current Bundle，并说明现有 Task 仍保持旧 pin

