# Media Core project

Status: target product specified; no Go application, HTTP API, Web UI or MCP implementation yet.

Media Core is a single person's persistent library of encountered, consumed and saved media. Its primary purpose is to answer what I watched/read, where I stopped and what I thought. Future interests, negative impressions and possible translation candidates are ordinary library information, not a rejection or recommendation workflow.

The user may know a title in Japanese, Russian, English, romaji or another alias. They should find an existing entry, see its editions, and follow explicit links to adaptations, translations and original publication sources. Missing metadata remains missing. An agent's guess about a source is a proposal until confirmed.

## Core example

An anime is completed and rated 8/10. Its manga source is `not_started` with intention `saved`, a publication URL and the note: “Interesting anime. Original is manga. Saved for later because reading manga on my phone is inconvenient.” Add `manga-translation-candidate`. Navigating either edition shows the relationship and retained personal history. A different device, edition or future translation may make reading practical later.

Works group a logical story; editions represent concrete publication/adaptation lines. The grouping is explicit and never proves a source relationship. A web novel need not be the original: manga, games, visual novels and anime-original productions can be roots. Uncertain origins stay uncertain.

## First usable product

Both Web UI and Hermes/MCP support manual add, duplicate review, multilingual search, work/edition details, progress/status updates, notes, ratings, intentions/tags, references and confirmed relationships. CLI uses the same services and provides import, backup, restore, export and diagnostics. No interface maintains separate rules or storage.

Local operations work offline without an LLM. Metadata providers and covers are optional future enrichment. Release Watcher owns detection; imported events never change personal reading progress. Translator Agent and future manga translation tools remain external.

## Runtime expectations

One Go binary with SQLite in a local state directory. CLI and MCP stdio run on demand; the optional HTTP/Web UI process is resident only while enabled. Web assets are built during development/release, embedded in Go, and require no production Node server. On a NAS, run processes on the NAS against its local filesystem; access the UI by SSH tunnel or authenticated TLS, and Hermes by stdio locally or over SSH.

See [scope](scope.md), [domain](domain-model.md), [contracts](application-contract.md), [architecture](architecture.md) and [roadmap](roadmap.md). Documentation is not implementation evidence.
