# Lightweight baseline build

This file records task 1.2 measurements from the detached baseline described in [environment.md](./environment.md). These values are denominators for the final candidate and do not prove that the target thresholds pass.

## Dependency installation

Command:

```bash
pnpm install --frozen-lockfile
```

Result: exit code `0`, `2011` packages installed with pnpm `10.28.2`. The install started from a worktree without `node_modules`. pnpm reported ignored optional dependency build scripts; the Web and Server production builds below still completed successfully.

## Server build

Source-of-truth command:

```bash
make build
```

The Make target built `server`, `multica`, and `migrate`; the release metric measures only `server/bin/server`. The Server build used:

```text
GOOS=darwin
GOARCH=arm64
CGO_ENABLED=1
Go toolchain=go1.26.1
version=736fbc8a
commit=736fbc8a
```

Result:

| Metric | Baseline | Final target |
| --- | ---: | ---: |
| Server binary bytes | `73853138` | `<= 51697196` (70%) |
| Server binary SHA-256 | `f6417273d8a42429e7849b2563e542ff69271d7d0a0037976490653a29bd1a1a` | Evidence only |
| Binary format | `Mach-O 64-bit executable arm64` | Same measurement environment |

The percentage limit is calculated using integer bytes and rounded down.

## Web production build

Command:

```bash
NEXT_TELEMETRY_DISABLED=1 pnpm --filter @multica/web build
```

This resolves to `fumadocs-mdx && next build --webpack` for `@multica/web@0.4.17`. Result: exit code `0` with Next.js `16.2.6`. The build emitted two existing CSS pseudo-element warnings and a non-fatal GitHub release API `403` while generating the download page.

The frozen Web JS set is every `*.js` file below `apps/web/.next/static`. The primary release denominator is the sum of individually gzip level-9 compressed files with filename/timestamp headers disabled. Raw bytes and the canonical set hash are supporting drift evidence.

| Metric | Baseline | Final target |
| --- | ---: | ---: |
| Client JS file count | `508` | Evidence only |
| Client JS raw bytes | `21143931` | Evidence only |
| Client JS gzip-9 bytes | `5200137` | `<= 3120082` (60%) |
| Canonical path+content digest | `28b2afa39a762139181083eeaf3532a00be362036685967882f13e515ce0515b` | Evidence only |

The canonical digest sorts paths relative to `.next/static`, appends each NUL-terminated path and the file's binary SHA-256, then hashes the stream. The percentage limit is calculated using integer bytes and rounded down.

## Reproduction commands

```bash
stat -f '%z' server/bin/server
openssl dgst -sha256 server/bin/server
find apps/web/.next/static -type f -name '*.js' | wc -l
find apps/web/.next/static -type f -name '*.js' -exec stat -f '%z' {} +
find apps/web/.next/static -type f -name '*.js' -exec gzip -9 -n -c {} + | wc -c
```

The raw-size command requires summing its output. Target measurements MUST reuse the exact file selection and gzip flags.

## Build-side mutation boundary

The detached source was clean before installation/build. Next.js rewrote tracked `apps/web/next-env.d.ts` during the build; no business source was edited. This generated mutation is excluded from baseline identity, which remains the clean recorded commit.
