# Lightweight baseline environment

This file records the clean reference environment required by task 1.1. It is evidence for comparison, not evidence that the target implementation or release gates have passed.

## Capture identity

- Captured at UTC: `2026-08-04T22:20:41Z`
- Captured at Asia/Shanghai: `2026-08-05T06:20:41+0800`
- Baseline commit: `736fbc8a5f1b22d48354a0e55baa00661e9e4326`
- Commit authored at: `2026-08-04T10:34:27+08:00`
- Commit subject: `MUL-5685: fix agent builder composer clearing and conversation routing (#6339)`
- Checkout mode: detached HEAD
- Checkout status: clean (`git status --porcelain=v1` produced no entries)
- Temporary checkout: `/tmp/dars-lightweight-baseline.plpoR9/worktree`

The temporary path is local evidence only and is not a stable release input. Reproduction MUST create a new detached worktree from the recorded commit.

## Host and toolchain

- OS: `macOS 15.2 (24C101)`
- Kernel: `Darwin 24.2.0 arm64`
- Hardware model: `Mac15,6`
- Logical CPUs: `11`
- Physical memory: `19327352832` bytes
- Host Go command: `go1.23.10 darwin/arm64`
- Module-selected Go toolchain: `go1.26.1`
- `GOTOOLCHAIN`: `auto`
- `GOOS/GOARCH`: `darwin/arm64`
- `CGO_ENABLED`: `1`
- Node: `v24.16.0`
- pnpm: `10.28.2`
- Git: `2.50.0`
- OpenSpec: `1.7.0`

The repository CI uses Node 22, while this local baseline uses Node 24.16.0. All target percentage comparisons MUST reuse this exact local environment or regenerate both baseline and target under the same CI-controlled Node 22 environment; values from the two environments MUST NOT be mixed.

## Frozen build inputs

| File | SHA-256 |
| --- | --- |
| `pnpm-lock.yaml` | `682f81a2740cb86efc567c9dc82f703c2bd20a6cf696dddefb4e23e16b99f8d5` |
| `server/go.mod` | `fbbc9ad411dc23e456f284ff4721728b98cab294b35d62302b3522c40555e8a1` |
| `server/go.sum` | `da961fd96b05ab6041749e23997a1cdde59d95e912c57caec94ed0e9fd4da` |
| `package.json` | `a0826c2d01295874d3f65c323eaa01801cbba164b681e72051c46bfafdce1f6e` |
| `turbo.json` | `ec36a09446532abc5c9962f70f3e4b919d2b89bcf458e8f409b35197537e8eb0` |
| `pnpm-workspace.yaml` | `af890d9063428d61318973ffe8a954b2ff7cfbc16ebfb9f17a85b7e6776b9feb` |

## Measurement contract

Baseline and target measurements MUST use all of the following unchanged unless both sides are regenerated:

- the same OS, architecture, hardware, Go/Node/pnpm toolchains and lockfiles;
- `CGO_ENABLED=1` and the normal production build commands from the recorded commit;
- a fresh dependency install from `pnpm-lock.yaml`;
- identical build environment variables and build tags;
- identical RSS warm-up, idle window and sample count;
- identical PostgreSQL readiness precondition and Server/Daemon readiness probes;
- raw artifact hashes and byte counts, not formatted filesystem sizes;
- explicit Router, Worker and schema snapshots from the measured process/database.

## Known pre-measurement state

- The detached worktree did not contain `node_modules` before installation.
- Available disk at capture time was approximately `143 GiB`.
- No business-code changes existed in the baseline worktree.
- Tasks 1.2 and 1.3 remain incomplete until the actual builds and runtime probes are captured.
