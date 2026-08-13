## Purpose

为 Lightweight Web、Server 与 CLI 提供可用的 Skills 上传、导入与管理工作流，覆盖手动创建、URL/zip 导入、Runtime local-skills 复制、文件管理与 Agent 绑定，且不恢复 Labels 或 Agent Template。

## ADDED Requirements

### Requirement: Skills 创建提供三种入口
Web Skills 表面 SHALL 提供互斥的三种创建入口：手动创建、Import from URL、Copy from runtime。手动创建 MUST 写入 workspace Skill 并允许随后编辑 `SKILL.md` 与附属文件；URL 与 Runtime 入口 MUST 进入对应导入流程而不是空壳占位。

#### Scenario: 用户打开 New skill
- **WHEN** 已登录成员在目标 Web 打开 Skills 创建入口
- **THEN** 可见 Manual / URL / Runtime 三种方式
- **THEN** 选择任一方式后进入可完成的创建或导入流程

### Requirement: URL 与 Archive 导入共用 Import API
Server SHALL 提供 `POST /api/skills/import`。JSON body 携带 `url` 时 SHALL 从 GitHub、ClawHub 或 Skills.sh 拉取并创建 Skill；`multipart/form-data` 上传 `.skill` 或 `.zip` 时 SHALL 解压校验后创建 Skill。冲突策略 SHALL 支持 `fail`、`overwrite`、`rename`、`skip`；overwrite 仅 Skill 创建者可用。导入结果 MUST 写入可被 Agent 绑定的 workspace Skill，并记录 origin 信息供列表展示。

#### Scenario: URL 导入成功
- **WHEN** 成员提交合法公开 Skill URL 且无同名冲突
- **THEN** Server 返回新建 Skill，文件集合完整可用

#### Scenario: Archive 上传成功
- **WHEN** 成员以 multipart 上传合法 `.skill`/`.zip` 且无冲突
- **THEN** Server 创建 Skill，附属文件路径均为相对路径且不含 traversal

#### Scenario: 同名冲突需策略
- **WHEN** 导入目标名称与现有 Skill 冲突且未提供可用 `on_conflict` 或策略为 `fail`
- **THEN** Server 拒绝盲目覆盖并返回冲突信息

### Requirement: ClawHub 搜索可用于发现导入源
Server SHALL 提供 `GET /api/skills/search?q=`，将查询转发到 ClawHub 并返回候选列表。上游不可用时 MUST 返回明确错误而不写入本地 Skill。CLI `multica skill search` MUST 调用该接口；Web 可将搜索结果衔接到 URL 导入，但 MUST NOT 把搜索本身当作创建完成。

#### Scenario: 搜索有结果
- **WHEN** 成员或 CLI 提供非空查询且上游可用
- **THEN** 返回候选列表，本地数据库无新 Skill 写入

#### Scenario: 上游失败
- **WHEN** ClawHub 请求失败或超时
- **THEN** API 返回上游不可用错误，不创建 Skill

### Requirement: Runtime local-skills 可发现并批量导入
Web SHALL 对 online Runtime 发起 local-skills 发现，展示可导入列表，并支持多选批量导入（含 conflict 时 overwrite/rename/skip）。该流程 MUST 复用已存在的 Runtime local-skills request/result API；仅 Runtime Owner 可发起 import。导入成功后 Skill 必须出现在 workspace Skills 列表并可绑定到 Agent。

#### Scenario: 从 online Runtime 复制
- **WHEN** Runtime Owner 选择 online Runtime、勾选本地 Skill 并确认导入
- **THEN** 系统完成 discovery → import → poll，新建或按策略更新 workspace Skill

#### Scenario: 非 Owner 不可导入
- **WHEN** 非 Runtime Owner 尝试发起 local-skill import
- **THEN** Server 拒绝该请求

### Requirement: Skill 文件管理可编辑完整文件集
Web Skill Detail SHALL 提供可读文件树与内容编辑，并通过现有 `PUT /api/skills/{skillId}/files` 原子替换完整文件集。路径 MUST 为相对路径；`SKILL.md` 为保留主文件。JSON textarea 不得作为唯一管理入口。

#### Scenario: 编辑附属文件后保存
- **WHEN** 有权限的成员修改文件树并保存
- **THEN** 文件集被完整替换，后续 get/list files 与绑定执行可见新内容

### Requirement: Skill 与 Agent 双向可绑定
成员 MUST 能从 Agent 配置绑定/解绑 Skill 并切换 enabled；也 MUST 能从 Skill Detail 将 Skill 添加到可管理的 Agent。绑定 API 继续使用全量 `PUT /api/agents/{agentId}/skills`。删除仍被绑定的 Skill MUST 返回冲突，UI SHALL 提示先解绑。

#### Scenario: 从 Agent 启用 Skill
- **WHEN** Agent 管理者勾选 workspace Skill 并保存
- **THEN** 后续 Task Claim 可解析到该 Skill bundle（在 enabled 时）

#### Scenario: 删除仍绑定的 Skill
- **WHEN** 成员删除仍被任一 Agent 引用的 Skill
- **THEN** Server 返回冲突且 Skill 仍存在

### Requirement: CLI 覆盖 import 与 search
`multica skill` SHALL 提供 `import`（`--url` 或 `--file`）与 `search` 子命令，并与 Web 共用同一 Server 契约与冲突策略语义。

#### Scenario: CLI 从文件导入
- **WHEN** 已登录 CLI 执行 `multica skill import --file ./example.skill`
- **THEN** 目标 Workspace 出现对应 Skill 或按 `on_conflict` 处理冲突
