## REMOVED Requirements

### Requirement: Router 精确匹配 114 条目标路由
**Reason**: Skills URL/zip 导入与 ClawHub 搜索重新进入目标产品，路由白名单需扩展 2 条。
**Migration**: 改用本变更新增的 116 条路由要求；同步更新 `contracts/routes.txt`、composition 与 release dump 门禁。

## ADDED Requirements

### Requirement: Router 精确匹配 116 条目标路由
Server SHALL 只注册按 `HTTP method + path template` 归一化后的 116 条路由，且不得注册重复路径、v1/v2 版本、旧 Desktop alias 或其他业务路由；query string 不计入 path template。

相对先前 114 条 Lightweight manifest，本变更新增：

```http
POST /api/skills/import
GET /api/skills/search
```

其余路由保持与 `multica-lightweight-runtime` 冻结清单一致，包括既有 Skill CRUD/files、Agent skills 全量 PUT，以及 Runtime local-skills list/import 与 daemon result 回调。完整清单以更新后的 contracts `routes.txt` 为准。

#### Scenario: Router snapshot 精确匹配
- **WHEN** contract test dump 全部已注册路由并按 method+path 排序
- **THEN** 结果与 116 条 manifest 精确相等且无重复
- **THEN** 清单包含 `POST /api/skills/import` 与 `GET /api/skills/search`

#### Scenario: 退出路由仍不可达
- **WHEN** 客户端访问未列入 116 条清单的旧路径（例如 Skill Labels）
- **THEN** Server 返回 404 且不进入旧 Handler

### Requirement: Skill Import 传输与体积边界
`POST /api/skills/import` SHALL 同时接受 `application/json`（URL 导入）与 `multipart/form-data`（archive 导入）。Archive 上传 MUST 强制压缩包大小上限与解压后的文件数/单文件/总大小上限；路径 MUST 拒绝 traversal 与绝对路径。JSON URL 导入 MUST 校验目标主机属于允许的公开 Skill 源，不得把任意内网 URL 当作导入源。

#### Scenario: 超限 archive 被拒绝
- **WHEN** 客户端上传超过压缩或解压上限的 archive
- **THEN** Server 返回 400 且不创建 Skill

#### Scenario: 非法路径被拒绝
- **WHEN** archive 或导入内容包含 `../` 或绝对路径文件条目
- **THEN** Server 拒绝导入
