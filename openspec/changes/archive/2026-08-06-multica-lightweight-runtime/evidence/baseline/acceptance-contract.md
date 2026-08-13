# Acceptance ID and release subset freeze

The authoritative rows in `04_e2e_acceptance_matrix.md` were parsed by the table-row grammar `ID + priority`. The result contains exactly 142 unique IDs:

- P0: 39
- P1: 100
- P2: 3
- families: 18

`contracts/acceptance-index.json` provides both an ordered case list and a unique `id_index`. Every case starts as `not_run` with an empty evidence list; the contract does not convert requirements into completion evidence. Allowed later states are `not_run`, `passed`, `failed` and `blocked`.

## Mandatory release subset

The required subset is the union of:

- all 39 P0 IDs;
- the frozen 15 security P1 IDs;
- `FLOW-01 FLOW-02 CM-07 RC-01 RC-02 TQ-01 TQ-03 TQ-04`;
- every `API-01..10`, `PRC-01..07`, `BLD-01..05` and `DB-01..10`.

After de-duplication, the release-required set contains 83 IDs. It is frozen in `contracts/acceptance-required.txt`.

The explicit security P1 set contains exactly:

`AUTH-02 AUTH-03 DR-11 SEC-03 SEC-04 SEC-05 SEC-06 SEC-07 SEC-08 SEC-09 SEC-10 SEC-11 WS-03 WS-04 WS-06`

All 15 exist in the matrix and are P1. The machine input is `contracts/acceptance-security-p1.txt`; release tooling must use this file instead of reclassifying safety scope.

## Validation and identity

- Source matrix SHA-256: `d734873273b1d4e364e746f5352a7098afbcb71c7ee8a84b820f7af6aa49ba2b`
- Acceptance index SHA-256: `ac17933a17a60d4a0d6f966f6a7278df69fbab9dd81fc8f110b52fb68c0e856c`
- Required subset SHA-256: `08ff363e76f4f1843641b32a5ce9e107a72cfe7b46bfee853d8ac4192c04582b`
- Security P1 subset SHA-256: `91a4db3981695aa66b215cf7b32b4c417451c81dfb6133669f0b6a05a1e6ff1e`

`jq` count, uniqueness, status and release-required assertions passed. Both text subsets pass ascending unique-order checks.
