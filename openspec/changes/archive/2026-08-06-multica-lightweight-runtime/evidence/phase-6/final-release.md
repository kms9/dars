# Phase 6：最终候选量化、Fresh Checkout 与发布判定

验证日期：2026-08-06（Asia/Shanghai）。

## 候选身份

- 基准/当前 HEAD：`736fbc8a5f1b22d48354a0e55baa00661e9e4326`。
- 候选形态：该 HEAD 上的当前 Lightweight 工作树补丁；尚未提交，因此二进制版本为 `736fbc8a-dirty`。
- OS/toolchain 与基准一致：macOS 15.2 arm64、Go 1.26.1、Node 24.16.0、pnpm 10.28.2。
- Server 构建：`make build`，CGO/default build contract 与基准一致。
- Web 构建：`NEXT_TELEMETRY_DISABLED=1 pnpm --filter @multica/web build`。

## 量化结果

| 指标 | 基线 | 最终候选 | 比例/门槛 | 结果 |
|---|---:|---:|---:|---|
| Server binary | 73,853,138 bytes | 32,160,898 bytes | 43.55% / <=70% | PASS |
| Web gzip-9 JS | 5,200,137 bytes | 324,430 bytes | 6.24% / <=60% | PASS |
| Server idle RSS mean | 30,321,869 bytes | 11,758,797 bytes | 38.78% / <=80% | PASS |
| Server Ready | n/a | 0.501632s | <=3s | PASS |
| Daemon Ready | n/a | 1.895808s | <=5s | PASS |
| Router | 351 baseline | 114 target | exact target | PASS |
| Application tables | 87 baseline | 26 target | exact target | PASS |

原始候选摘要：

```text
server_sha256=b276dd3b4b3079377807586c0be2a428cefec483d512a578c45bd984d56fef90
web_js_file_count=47
web_js_raw_bytes=1045827
web_js_canonical_digest=eb74e6201404424a01cc5e13dddfd299b623f19a2c4d894e8a91405ba462f9b7
rss_samples_kib=11488,11472,11488,11488,11488,11488,11488,11488,11472,11472
```

## Fresh Checkout `make check`

从 `/tmp` 创建 HEAD 的 detached worktree，叠加当前 tracked binary diff 与全部未跟踪候选文件，复制忽略提交的本地测试 `.env`，执行：

```text
pnpm install --frozen-lockfile
CHECK_DATABASE_SUFFIX=fresh_candidate_2 make check
```

结果：

```text
typecheck: 4 target packages pass
Web production build: pass
TypeScript tests: Core 38 + Views 23 + Web 58 = 119 pass
Go tests: all target packages pass against fresh migrated database
Chromium E2E: 1 pass
fresh_candidate_exit=0
frontend_port_released=yes
backend_port_released=yes
temporary_checkout_removed=yes
```

首次 Fresh Checkout 重跑曾发现 `make check` 只停止 pnpm wrapper、遗留 Next 子进程的问题。最终候选改为为 backend/frontend 建立独立 process group，并在 cleanup 中终止整个 group；第二次 Fresh Checkout 证明两个端口均释放。该失败记录没有被误计为通过。

## 数据与回滚边界

- 运行数据库：Docker PostgreSQL 17 的独立 `multica_lightweight`，持久化 volume `multica_pgdata`。
- Check 数据库：每次创建独立 `multica_lightweight_check_*`，结束后强制删除。
- 原完整产品 `multica`：保持只读保留，不 upgrade、不 downgrade、不双写。
- 回滚发行物：基准 Server hash、最终候选 Server hash、Web digest 与独立 DSN 均已记录；失败候选数据库只读保留或显式重建，不把 Lightweight schema 回灌旧库。

## 发布判定

代码、量化、Fresh Checkout、真实浏览器与真实 Runtime 门槛均已通过。142 项索引中除 DB-07 外均有通过证据；DB-07 因最终旧库 checksum 无可用安全凭据而 `blocked`。因此：

```text
passed=141
blocked=1 (DB-07)
failed=0
not_run=0
release_approved=false
```

只有完成任务 13.8 的旧库只读 revision/schema/data checksum 复核且与基线一致后，才能勾选 14.9、14.12 并批准发布。

