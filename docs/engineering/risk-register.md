# Risk register

Target mitigations are planned, not verified runtime properties.

| Risk | Consequence | Mitigation / owner |
| --- | --- | --- |
| Similar titles mistaken for identity | Lost distinctions and wrong sources | Explicit candidate decisions, exact reference constraints, transactional recheck; 0012/0044 |
| Work grouping mistaken for original source | False WN-first or anime/manga assumptions | Separate edition edges and confirmed origin claims, proposals labeled; 0020/0033/0041 |
| Personal/publication/intention conflated | Saved becomes dropped or releases become read | Independent columns/history/services and cross-interface counterexamples; 0014–0016/0043/0046 |
| Unicode matching inconsistent | Alias lookup misses or duplicates | Stored NFKC/case-fold key; literal wildcard and RU/JP fixtures; 0010/0013 |
| SQLite multi-process/restore race | Lost commits or split live DB handles | Lifetime shared lock, exclusive maintenance, WAL/busy timeout, interrupted restore tests; 0008/0022/0023/0044 |
| Browser/LAN exposure | Unauthorized writes/secret leakage | Mandatory owner sessions, exact Host/Origin/CSRF, explicit TLS remote config; 0024/0030/0047 |
| Provider guesses become facts | Incorrect graph/metadata | Proposal/confirmation provenance, manual fallback, no provider dependency; 0020/0041 |
| Import drift/replay/conflicting event | Duplicate or wrong release history | Pinned 1.0 fixtures, immutable payload, event conflict and binding audit; 0042/0043 |
| Web and MCP drift | Different identity/history semantics | Frozen shared DTOs/use cases, parity and scenarios A–D; 0007/0045/0046 |
| Broad task or merge contention | Agents infer architecture, overwrite files | Named capability files, prerequisite contracts, isolated worktrees, narrow task scopes; implementation plan |
| Pure-Go driver/SDK release choices | Compatibility/security uncertainty | Check supported versions at implementation; pin and record decisions; 0008/0037 |
| NAS Hermes cannot launch SSH subprocess | Remote workflow unavailable | Verify client config in 0048; native remote MCP requires a separate approved transport/auth task, local MCP remains usable |
| No license selected | Unclear redistribution rights | Operator choice remains open; packaging docs must flag it, no agent invents license |
| Interrupted archive/insufficient disk | Corrupt backup or overwritten data | Temp paths, bounds/hash/integrity, atomic publish and rollback; 0022/0023 |

Recommended defaults are documented, not left to coding agents: same-owner auth, SSH bridge, no metadata provider, local-disk DB, confirmed explicit graph. Exact dependency versions, TLS certificate provisioning and client-specific Hermes config are implementation-time environment checks; they do not block the documentation graph. No actual NAS deployment is authorized by these tasks.
