# Hermes / MCP design

Hermes is a first-class MVP client. `media-core mcp` exposes MCP stdio over the same in-process application services as Web/CLI; no HTTP prerequisite and no generic command or SQL tool. Protocol initialization, capabilities and framing use a maintained Go MCP SDK selected/pinned with an ADR dependency note in task 0037. Pin a mutually supported protocol version in the smoke fixture then; do not assume a particular Hermes installation already supports it.

## Typed tool surface

| Task | Tools | Behavior |
| --- | --- | --- |
| 0038 | `search_library`, `get_work`, `get_edition` | Shared bounded filters and DTOs |
| 0039 | `resolve_addition`, `create_entry`, `update_catalog` | Explicit duplicate decision; add edition/title or metadata correction/archive |
| 0040 | `record_progress`, `set_consumption`, `get_history` | Append or explicitly correct/retract; expected revisions and idempotency |
| 0041 | `set_intention`, `set_tag`, `write_note`, `record_rating`, `write_reference`, `list_related`, `write_relation`, `write_origin`, `list_activity` | Typed action enums for add/revise/retire, propose/confirm/revoke; shared validation |
| 0043 | `list_imports`, `import_releases`, `bind_import`, `list_releases` | Bounded JSON event batches; no arbitrary filesystem reads |

Each write description says it mutates the library. Set MCP read-only/destructive annotations truthfully; all tools accept narrow JSON schemas with required fields, length/enumeration constraints and additionalProperties false. `update_catalog` actions are edition metadata, title replacement or archive, not arbitrary field writes. `write_reference` supports add/retire; `write_note` add/revise/retract; `get_history` includes correction state. Errors retain the app code/retryable/details in structured tool results; tool success never hides failure.

Agent-found relations/origins default to proposed with source provenance. Confirm requires `confirmation:true`, expected revisions and user-instruction context. This is an explicit intent guard, not a claim that the server can infer whether an LLM is truthful; the OS-authorized Hermes principal is trusted like the user's CLI. A confidence number is never confirmation. No network metadata calls, shell, URL acquisition, translation execution, backup path selection or SQL through tools.

Examples: “Добавь эту мангу ... прочитать позже” maps to resolve/create plus intention saved; “Я закончил читать 42-ю главу” appends chapter 42, without guessing completed status; “Какие аниме я уже посмотрел?” filters kind anime + consumption completed; translation candidates filter the tag. Related queries traverse confirmed source edges; proposals are separately labeled.

## Connection topologies

1. Same machine: Hermes launches the absolute binary path with args `["mcp"]` and `MEDIA_CORE_STATE_DIR` pointing to the initialized local state directory. No listener, token or HTTP dependency. Stdout is MCP only; logs to stderr.
2. Hermes on another host/NAS library: launch `/usr/bin/ssh` as the local stdio subprocess with args such as `["-T", "-o", "BatchMode=yes", "media-core-nas", "/usr/local/bin/media-core", "mcp"]`. The SSH host alias supplies a dedicated account/key and verified host key; the account's environment/default XDG directory supplies state. Do not concatenate user titles or paths into a remote shell command. Prefer a restricted key/forced command limited to the fixed binary and state directory, not arbitrary shell access. No PTY or login banners on stdout. Binary/DB remain on the NAS; SSH supplies authenticated encrypted transport. Task 0048 validates actual Hermes config shape on the installed client and supplies a clearly labeled example; it must not claim a real connection was tested unless it was.
3. Native remote MCP: deferred. Stdio alone is not a remote protocol; SSH is the selected bridge. If a client cannot launch subprocesses, add Streamable HTTP later with explicit authentication, TLS and Origin checks. Browser sessions are not automatically a suitable MCP authorization system; decide and document the MCP authorization flow before implementing that adapter.

The [MCP transport specification](https://modelcontextprotocol.io/specification/2025-11-25/basic/transports) distinguishes stdio and Streamable HTTP. The initial product only implements stdio. `serve` never silently enables a remote `/mcp` endpoint.

## Smoke contract

Task 0048 starts the built subprocess against a temporary DB, initializes MCP, lists tools, creates an entry, searches a Russian/romaji alias, records progress/status, checks history, restarts and verifies the same IDs/state. A deterministic local SSH fixture may check the remote-command framing; actual NAS access requires separate operator authorization. Failure/EOF must cleanly release DB handles. No deployment to a real host is authorized by planning.
