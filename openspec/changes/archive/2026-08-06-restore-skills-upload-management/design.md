## Context

See proposal.md for motivation. Current Lightweight Web Skills UI is a minimal form/list in `packages/views/lightweight/skills.tsx`. Server already exposes Skill CRUD/files and Runtime local-skills request/result routes; URL/zip import and ClawHub search were removed from the 114-route whitelist.

Authoritative reference implementation (do not copy Desktop/labels/template paths):

- `/Users/logo/self_repo/video_case/fx_coding_case/multica/packages/views/skills/components/create-skill-dialog.tsx`
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/packages/views/skills/components/runtime-local-skill-import-panel.tsx`
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/packages/views/skills/components/skill-detail-page.tsx` (+ file tree/viewer)
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/packages/core/runtimes/local-skills.ts`
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/server/internal/handler/skill.go` (`ImportSkill`, `SearchSkills`)
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/server/internal/handler/skill_import_archive.go`
- `/Users/logo/self_repo/video_case/fx_coding_case/multica/server/cmd/multica/cmd_skill.go`

Constraints: Lightweight target only (Web + Server + Daemon/CLI); keep full-set skill files replace and full-set agent skills PUT; no Labels/Template; Desktop out of scope; prefer slim components under `packages/views/lightweight` over reviving whole `packages/views/skills`.

## Goals / Non-Goals

**Goals:**

- Restore both import tracks needed by follow-on workflows: Runtime local-skills copy, and URL/zip upload.
- Make Skill management usable again: create chooser, file tree, binding, list search.
- Expand frozen route manifest 114 → 116 with import/search, and keep contracts/tests green.
- Port behavior from the reference repo into `lightweightapi` / lightweight core/views, not by re-enabling old Desktop handler packages wholesale.

**Non-Goals:**

- Skill Labels, Agent Template/Builder, marketplace browsing beyond ClawHub search → URL import.
- Incremental agent-skill endpoints (`skills/add`, per-id enable/delete routes).
- Restoring Redis-backed local-skills store if Lightweight already uses in-memory request control (keep current Lightweight request model).
- Desktop/Mobile UI parity.

## Decisions

### 1. Port import/search into `lightweightapi`, do not resurrect old handler package as a dependency
- **Choice**: Re-implement `ImportSkill` / archive parse / `SearchSkills` inside `server/internal/lightweightapi` (and shared pure helpers under `server/internal/skill` if still present or ported), wired through `lightweight_composition.go`.
- **Why**: Lightweight already cut over away from `handler.Handler` for target routes; pulling old handler package back would reintroduce exited surface area.
- **Alternatives**: Call into copied old handler files via adapter — rejected (boundary leak). Proxy to external service — rejected (offline/self-host requirement).

### 2. Expand contracts explicitly (116 routes)
- **Choice**: Add `POST /api/skills/import` and `GET /api/skills/search` to `routes.txt`, transport schemas, composition, and release dump expectations.
- **Why**: Both follow-on flows need them; zip and URL share one POST with content-type branching (reference behavior).
- **Alternatives**: Only CLI-side parsing without Server import — rejected (Web upload needed). Separate `/api/skills/import-archive` — rejected (breaks reference/CLI contract).

### 3. Slim Web port of the three-way create dialog + local-skills panel
- **Choice**: Implement lightweight-specific components under `packages/views/lightweight/` (e.g. `skills-create.tsx`, `skills-runtime-import.tsx`, `skills-files.tsx`), adapting reference UX but using `@multica/core/lightweight` and existing UI primitives. Avoid motion-heavy / ListGrid / labels dependencies.
- **Why**: Bundle budget and package boundary; reference panel is large (~1.3k lines) and pulls runtime-machines/i18n/search helpers not all present in Lightweight.
- **Alternatives**: Vendoring entire `packages/views/skills` — rejected for size and exited deps.

### 4. Local-skills client helpers live in `packages/core/lightweight`
- **Choice**: Port poll helpers equivalent to reference `packages/core/runtimes/local-skills.ts` into lightweight core (initiate/list/import + timeout constants aligned with server pending/running timeouts).
- **Why**: API client for Lightweight is already split; do not revive full `@multica/core/runtimes` if it is outside target graph.
- **Alternatives**: Inline polling only in the view — rejected (harder to test/reuse from Agent page later).

### 5. File management uses tree editor over full-set replace
- **Choice**: UI edits a local file map, then submits complete array to existing `replaceSkillFiles`. Optional single-file delete uses existing delete endpoint or folds into replace.
- **Why**: Matches current Server contract; no incremental file PATCH needed.
- **Alternatives**: Reintroduce per-file PATCH APIs — rejected (unnecessary contract growth).

### 6. Binding stays on full-set PUT
- **Choice**: Agent skills UI supports enabled toggles by rewriting the full binding set; Skill detail “add to agents” loads agents, merges bindings, PUTs per agent.
- **Why**: Incremental routes are intentionally out of the Lightweight whitelist.
- **Alternatives**: Re-add `POST .../skills/add` etc. — rejected for this change.

### 7. Reference conflict/origin semantics preserved
- **Choice**: Keep `on_conflict` strategies and origin metadata (`manual` / `runtime_local` / `clawhub` / `skills_sh` / `github` / archive) in Skill `config` where the Lightweight schema already allows opaque config.
- **Why**: Follow-on workflows and list filters depend on knowing where a skill came from.
- **Alternatives**: Drop origin — rejected (hurts later ops/debug).

## Risks / Trade-offs

- [114→116 freeze drift vs `multica-lightweight-runtime`] → Treat this change as the explicit reopen; update sibling contracts/evidence in the same implementation PR or coordinated PR pair; do not silently edit only one side.
- [SSRF / archive zip-slip on import] → Port allowlisted hosts, size caps, and path validation from reference tests; add Lightweight-focused security tests.
- [Reference UI too heavy for JS budget] → Slim rewrite; measure `pnpm build` web JS against release threshold.
- [Local-skills Owner-only vs workspace Admin expectations] → Keep Owner-only import semantics; surface clear empty/disabled state for non-owners.
- [ClawHub upstream flakiness] → Search returns upstream error; import path remains usable with pasted URL/file even if search fails.
- [Parallel with unfinished lightweight change] → Implementation must not reintroduce exited APIs; only the two approved skill routes.

## Migration Plan

1. Land Server import/search + contract updates; prove route dump = 116.
2. Land core helpers + CLI import/search.
3. Land Web create/import/file/binding UX behind normal Lightweight routes (no feature flag).
4. Add e2e coverage for at least one URL or archive import and one runtime import (runtime import may use fake daemon result in API tests + one browser smoke if daemon available).
5. Rollback: revert route registration and UI entry points; no DB migration expected if Skill/skill_file tables already exist.

## Open Questions

- Web Skills 页是否在 P0 就内嵌 ClawHub search UI，还是 P0 只做 URL 粘贴 + CLI search（archive/runtime 仍是 P0）？默认：**P0 Web 做 URL 粘贴 + 文件上传；search UI 可作为同页增强，但不阻塞 import API。**
- 与 `multica-lightweight-runtime` 未完成任务的合并策略：同 PR 更新 contracts，还是本 change 单独维护一份 routes overlay？默认：**实现时直接更新共享 contracts/routes.txt 与 composition，并在两个 change 的 evidence/tasks 中交叉引用。**
