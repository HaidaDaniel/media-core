# Load profile

Planning assumptions, not measurements: one owner, up to 10,000 works, 30,000 editions, 100,000 titles and about 1 million accumulated history/import rows over years. Daily use is tens of writes and hundreds of reads; occasional import up to 1,000 events per CLI batch, HTTP/MCP batches constrained by 1 MiB and at most 100 events. No scraping workload.

HTTP may remain resident alongside one Hermes process and occasional CLI commands. Start with one SQLite connection per process, WAL and short writes; bounded queries and cursor pages of 30/max 100. History and activity use edition/time indexes. Initial normalized substring search can scan title rows; instrument before adding FTS. Never sum publication-line counts or cross-edition progress.

Targets to measure later on a documented representative NAS: ordinary reads/search p95 under 300 ms on a warm 10k-work fixture; local mutations p95 under 500 ms outside contention; bounded busy wait 5 seconds. Resident RSS goal under 100 MiB before large imports is provisional. CLI/HTTP-disabled modes consume no resident resources; active serve/MCP do. No fabricated performance results or benchmark requirement for documentation.

Backup time/disk growth depend on DB size; require free space for snapshot + archive + restore rollback copy. Import validates a bounded envelope and commits per event; exports stream ordered rows. Concurrency tests verify no partial observations and deterministic dedupe, not production throughput claims. Larger workloads trigger measurement and a separate decision, not an automatic move to Redis/PostgreSQL.
