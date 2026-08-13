# Route contract freeze

The first `http` code block under `Router 精确匹配 114 条目标路由` in `lightweight-api-security/spec.md` was parsed as the authoritative source. Blank lines were discarded, each non-empty line was validated against an allowed HTTP method plus absolute path template, duplicates were rejected, and the result was sorted in ascending byte order.

Validation result:

- source entries: `114`
- unique normalized entries: `114`
- malformed entries: `0`
- fixture lines: `114`
- fixture SHA-256: `25863725bd9232d8e730f7b11dff7cac93bc2cb786cdd42cc9efc12651b0ea7e`
- exact set difference between source block and fixture: empty

Machine inputs are `contracts/routes.txt` and `contracts/routes.meta.json`. Later Router contract tests SHALL compare the live Chi walk to this fixture as an exact set, not as a subset.
