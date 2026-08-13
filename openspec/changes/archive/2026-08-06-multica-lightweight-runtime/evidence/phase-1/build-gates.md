# Phase 1 BLD-01..05 evidence

> 2026-08-06 终态更新：本文件保留 Phase 1 的历史红灯；最终 Fresh Checkout 的 BLD-03 已通过，见 `../phase-6/final-release.md`。

Captured on 2026-08-05 (Asia/Shanghai) for task 2.6. This is an implementation-stage gate report, not final release approval.

| ID | Status | Evidence |
| --- | --- | --- |
| BLD-01 | PASS | Root `pnpm build` exited 0. Turbo dry-run executable package set was exactly `@multica/web`; Core/UI/Views have no build command. Desktop, Mobile and Docs were absent. |
| BLD-02 | PASS | Root typecheck exited 0 for Web/Core/UI/Views; Vitest passed Web 188 + Core 1234 + Views 3485 = 4907 tests; lint exited 0 with 16 pre-existing warnings and no errors. Dry-run executable sets contained only target packages. |
| BLD-03 | **FAIL** | The isolated `make check` reached all six stages and passed target TypeScript, Web build/contracts, migrations and all Go tests. Playwright finished 22/30 with eight failures, so the acceptance gate remains failed. See [check-orchestration.md](./check-orchestration.md). |
| BLD-04 | PASS (local source contract) | `apps/web/build-graph-contract.test.ts` passed 4/4. It proves required CI positively selects only Web/Core/UI/Views and the shared-package path gate does not introduce Desktop/Mobile/Docs. No remote GitHub run was executed for this local evidence. |
| BLD-05 | PASS | CLAUDE, AGENTS, root CONTRIBUTING and the four developer-contributing documents now describe the Lightweight target boundary. Docs typecheck exited 0 and the obsolete dual-platform-required wording scan returned no matches. |

## Positive build graph evidence

Turbo `--dry=json` executable package sets:

```text
build      @multica/web
typecheck  @multica/core,@multica/ui,@multica/views,@multica/web
test       @multica/core,@multica/views,@multica/web
lint       @multica/core,@multica/ui,@multica/views,@multica/web
```

`@multica/ui` has no test command, so Turbo correctly emits no executable UI test task. Hash-only configuration nodes are not executable tasks.

## Desktop source boundary

The following checks were empty/successful after all Phase 1 edits:

```text
git diff --quiet HEAD -- apps/desktop   # exit 0
git status --porcelain=v1 --untracked-files=all -- apps/desktop   # empty
HEAD:apps/desktop tree = d8456bf428c6e94562a1abad37092d088e3156b9
```

This confirms the current Phase 1 worktree did not actively modify tracked or untracked Desktop-specific source. It does not claim that Desktop remains compatible with the breaking Lightweight target.

## Gate conclusion

Task 2.6's evidence capture is complete, but BLD-03 is still red. Final tasks 10.14 and 14.8 must replace obsolete Full-product browser coverage with the target suite and produce a fully passing fresh-checkout run before release.
