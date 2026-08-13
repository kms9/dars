## MODIFIED Requirements

### Requirement: 路由、Worker 和数据表计数是硬门禁
发布 SHALL 要求 Router dump 精确等于 116 条 manifest、运行 Worker 精确等于批准清单、`pg_catalog` 显示恰好 26 张应用表。页面减少、单元测试通过或自描述 health 响应不得替代这些证据。

#### Scenario: 任一清单漂移
- **WHEN** Router 多/少一条路径、运行中出现额外 Worker，或数据库应用表不是 26 张
- **THEN** 发布门禁失败

#### Scenario: Skills 导入路由计入门禁
- **WHEN** 发布流程 dump Router
- **THEN** 结果包含 `POST /api/skills/import` 与 `GET /api/skills/search`
- **THEN** 总数精确为 116

## ADDED Requirements

### Requirement: Skills 上传与管理进入发布验收
发布验收 SHALL 覆盖 Skills 手动创建、URL 导入、archive 上传、Runtime local-skills 导入、文件编辑与 Agent 绑定的正向路径；并覆盖非法 archive、非 Owner local import、删除仍绑定 Skill 的负向路径。

#### Scenario: Skills 导入闭环
- **WHEN** 在真实独立 Server/Daemon/Web 环境执行 URL 或 archive 导入，以及 Runtime local-skills 导入
- **THEN** workspace Skills 列表出现结果，且可绑定 Agent 后被 Task 执行解析
