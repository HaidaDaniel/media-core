# MCP client connections (Hermes and any agent)

media-core exposes MCP over two transports, both built on the same in-process application services as Web/CLI: no HTTP prerequisite, no generic command, shell, file or SQL tool. Protocol initialization, capabilities and framing use a maintained Go MCP SDK selected/pinned with an ADR dependency note in task 0037. Pin a mutually supported protocol version in the smoke fixture; do not assume a particular client installation already supports it.

| Transport | Command | Auth | Use |
| --- | --- | --- | --- |
| stdio | `media-core mcp --state-dir <dir>` | none (the client owns the process) | same-host agents: Hermes, Claude Desktop, opencode, any subprocess-capable client |
| Streamable HTTP (opt-in) | `media-core mcp --http --listen <addr> [--allow-remote] [--public-origin <url>] [--tls-cert <file>] [--tls-key <file>]` | `Authorization: Bearer <owner-secret>` | clients that cannot launch subprocesses; remote hosts |

Stdout on stdio is MCP only; logs go to stderr. `serve` never silently enables an MCP endpoint: the web listener carries no `/mcp` handler.

## Typed tool surface

| Task | Tools | Behavior |
| --- | --- | --- |
| 0038 | `search_library`, `get_work`, `get_edition` | Shared bounded filters and DTOs |
| 0039 | `resolve_addition`, `create_entry`, `update_catalog` | Explicit duplicate decision; add edition/title or metadata correction/archive |
| 0040 | `record_progress`, `set_consumption`, `retract_progress`, `get_history` | Append or explicitly correct/retract; expected revisions and idempotency |
| 0041 | `set_intention`, `set_tag`, `write_note`, `record_rating`, `write_reference`, `list_related`, `write_relation`, `write_origin`, `list_activity` | Typed action enums for add/revise/retire, propose/confirm/revoke; shared validation |
| 0043 | `import_releases`, `bind_release`, `rebind_release`, `revoke_release`, `list_releases`, `list_pending_imports`, `get_release_binding` | Bounded JSON event batches; no arbitrary filesystem reads |

Each write description says it mutates the library. Set MCP read-only/destructive annotations truthfully; all tools accept narrow JSON schemas with required fields, length/enumeration constraints and additionalProperties false. `update_catalog` actions are edition metadata, title replacement or archive, not arbitrary field writes. `write_reference` supports add/retire; `write_note` add/revise/retract; `get_history` includes correction state. Errors retain the app code/retryable/details in structured tool results; tool success never hides failure. Write tools take `expected_revision` from the current view: read `get_work`/`get_edition` first, and a stale revision is a conflict that leaves the prior state intact. The same `request_id` with the same payload replays the stored result.

Agent-found relations/origins default to proposed with source provenance. Confirm requires `confirmation:true`, expected revisions and user-instruction context. This is an explicit intent guard, not a claim that the server can infer whether an LLM is truthful; the OS-authorized agent principal is trusted like the user's CLI. A confidence number is never confirmation. No network metadata calls, shell, URL acquisition, translation execution, backup path selection or SQL through tools.

Examples: “Добавь эту мангу ... прочитать позже” maps to resolve/create plus intention saved; “Я закончил читать 42-ю главу” appends chapter 42, without guessing completed status; “Какие аниме я уже посмотрел?” filters kind anime + consumption completed; translation candidates filter the tag. Related queries traverse confirmed source edges; proposals are separately labeled.

## Owner-secret authentication

The Streamable HTTP transport authenticates with the state directory's owner secret — the same secret the web login uses, no second secret and no browser session:

- `media-core owner generate --state-dir <dir>` creates `<dir>/owner-secret` (mode 0600) and prints its hex value once; `owner show` prints it again.
- Every request must carry `Authorization: Bearer <hex>`, compared in constant time. A missing secret file is a distinct 401 ("not configured; run media-core owner generate"), and a missing or wrong token is a 401 that never reaches the protocol — rejected requests touch no state.
- The listener is loopback by default. A non-loopback address is refused at configuration time unless the explicit remote trio is present (`--allow-remote`, an https `--public-origin`, `--tls-cert` and `--tls-key`); TLS then carries the Bearer token over the wire.
- Sessions are stateless per request over the same in-process service: no second database, no network-attached DB, no per-client state to clean up.

## Connection topologies

1. Same machine, any client: the client launches the absolute binary path with args `["mcp"]` and `MEDIA_CORE_STATE_DIR` (or `--state-dir`) pointing at the initialized local state directory. No listener, token or HTTP dependency.
2. Same machine or remote, subprocess-less client: run the Streamable HTTP mode (above) and point the client at `http://127.0.0.1:8787/mcp` (the default loopback listener) with the Bearer header. For another host/NAS library, bind with the explicit remote trio over TLS and keep the binary and database on that host.
3. Remote over SSH (alternative for stdio-capable clients): launch `/usr/bin/ssh` as the local stdio subprocess with args such as `["-T", "-o", "BatchMode=yes", "media-core-nas", "/usr/local/bin/media-core", "mcp"]`. The SSH host alias supplies a dedicated account/key and verified host key; the account's environment/default XDG directory supplies state. Do not concatenate user titles or paths into a remote shell command. Prefer a restricted key/forced command limited to the fixed binary and state directory, not arbitrary shell access. No PTY or login banners on stdout. Binary/DB remain on the NAS; SSH supplies authenticated encrypted transport.

### Client configuration examples

**Pending actual client validation**: no MCP client (including Hermes) is installed on the machine where these examples were written, so the shapes below are the generic MCP client config forms, not a tested connection. Validate the config against the installed client before relying on it; a real-client or NAS check is reported as unrun unless it was actually authorized and performed.

Generic stdio (Claude Desktop-style JSON; Hermes or any client with the same shape):

```json
{
  "mcpServers": {
    "media-core": {
      "command": "/usr/local/bin/media-core",
      "args": ["mcp"],
      "env": { "MEDIA_CORE_STATE_DIR": "/home/user/.local/share/media-core" }
    }
  }
}
```

Generic Streamable HTTP:

```json
{
  "mcpServers": {
    "media-core": {
      "url": "http://127.0.0.1:8787/mcp",
      "headers": { "Authorization": "Bearer <owner-secret-hex>" }
    }
  }
}
```

Remote via SSH (stdio over an encrypted bridge; the client still launches a local subprocess):

```json
{
  "mcpServers": {
    "media-core-nas": {
      "command": "/usr/bin/ssh",
      "args": ["-T", "-o", "BatchMode=yes", "media-core-nas", "/usr/local/bin/media-core", "mcp"]
    }
  }
}
```

## Smoke contract

The acceptance suite (task 0048) drives the built binary the way an external client would:

- stdio: initialize, `tools/list`, `create_entry` (work + edition via a known identity), idempotent replay of the same `request_id`, `search_library` by the romaji alias, view-read then `record_progress`, `get_history` — then a hard kill, a restarted client, and the same IDs/state; the killed process must have released the database handle, and a clean client EOF must exit without error. No HTTP, Node or provider is present in that environment.
- Streamable HTTP: the same lifecycle over loopback with the Bearer secret; a missing secret file is the distinct 401; a missing or wrong token is a 401 with the state untouched; a non-loopback listen without the trio fails before any listener; the TLS listener serves the same lifecycle; and `serve` exposes no MCP endpoint.

No deployment to a real host is authorized by planning; actual NAS access requires separate operator authorization.
