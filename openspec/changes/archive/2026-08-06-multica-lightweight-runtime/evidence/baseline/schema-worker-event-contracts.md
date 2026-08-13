# Schema, Worker and event contract freeze

The machine contracts were generated from the frozen OpenSpec requirements and the authoritative database retention PRD. No live implementation state was used to expand the target allowlists.

## Source identities

- Database retention PRD SHA-256: `c0b5d3d9fa92a8a13dbd85b7bef0cfcde85a723e5548746af3c9f5d6f9d53c63`
- Data baseline spec SHA-256: `078635b8a741e32a73732ac4038bf369e29ca1435a79c326d5ea2fd7f8ac6f34`
- API security spec SHA-256: `30cb371dbb76f1b47e9b5a081ec86db6e2c3110d68799ac7593da2162f945969`
- Runtime release spec SHA-256: `2da53e412aa770cefcd7315a4d798ec7683b3adf090f70d68817decb0b9d71d3`

## Schema contracts

- `tables.txt`: exactly 26 sorted, unique application tables; its set and order match both the PRD and OpenSpec table block.
- `schema-columns.tsv`: 281 unique table+column rows across exactly 26 tables. It expands the PRD shorthand into PostgreSQL types, nullability, defaults and enum values.
- `schema-constraints.json`: no FK/CASCADE policy, DDL/index policy, 14 row checks and the cross-table Service invariants.
- `schema-indexes.json`: 72 semantic index fingerprints: 45 unique indexes, including 25 ID indexes, and 27 access indexes. Physical names are deliberately excluded; audits compare table, ordered keys, order direction, uniqueness, predicate and NULLS NOT DISTINCT.

The constraints contract is semantic. Enum CHECK inputs are carried by the `enum` column in `schema-columns.tsv`; unique rules are carried by `schema-indexes.json`; cross-table rules remain Service transaction invariants because the target forbids database foreign keys.

## Runtime contracts

- `workers.json`: 3 default Workers, 2 configuration-gated Workers and 11 explicitly forbidden Worker families.
- `web-events.txt`: 31 sorted, unique Workspace event types.
- `daemon-events.txt`: 8 sorted, unique Daemon control event types.

## Validation

All JSON files parsed with `jq`. Count assertions, duplicate table+column checks, source-set comparisons and ascending unique-order checks passed. Contract hashes:

| Contract | SHA-256 |
|---|---|
| `tables.txt` | `c4c8dec7a93a12d010b089fbdf0203937a6a6033dd1e0c45dd5de884eceda6ad` |
| `schema-columns.tsv` | `0765258a9f33b6a2c109cf3d425afcef360232e182a914a38b86917814bed53e` |
| `schema-constraints.json` | `dd3cb4c9f072678967b1ac5e80e99c949eed697ae3c73dc7b1cc8a7f17b5af9b` |
| `schema-indexes.json` | `1e74fa3dcf19d30ed6cd98b6074d7a0542ec3daa5fa13066891cd3633c52367b` |
| `workers.json` | `94afa66b5c4ae1cee21f1c55d6d6c64cb7bc41f531fec875875e4c41bab3f1a9` |
| `web-events.txt` | `ac4f93d2e0ba6d987125f2e61539b1be54186c258431aa983c2081752b63535c` |
| `daemon-events.txt` | `ab4d6ba0893ae989f4229b53a7f7c23b4944adde918a663bf0b61d092e23f032` |
