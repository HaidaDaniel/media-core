# Deployment

This document describes how to build the release artifacts and run
media-core on a host. media-core is a single static Go binary: the HTTP
API, the embedded web UI, the CLI and the MCP stdio server all live in it.
There is no runtime dependency on Node, pnpm or Docker; the only
filesystem requirements are the state directory and, for remote access,
TLS certificate files.

## Release artifacts

`scripts/build-release.sh` builds the complete release. It is the only
step that needs a toolchain beyond Go: the frontend build runs
`pnpm --dir web exec vite build`, after which the script stages and
validates the assets before embedding them. The script:

1. Builds the frontend with Vite.
2. Stages the build in a temporary directory and validates it: the index
   shell is non-empty, it references no external `http(s)` resources, and
   every local reference resolves to a staged file. A manifest with each
   file's name, size and SHA-256 is written alongside and then verified at
   runtime against the real staged files: complete file set, no self-entry,
   matching sizes and hashes. The build script runs the gate itself with
   absolute staging paths: the manifest check while staged files exist, and
   `MEDIA_CORE_RELEASE_GATE=1 go test ./tests/packaging -run TestPackageReleaseArtifacts`
   after linking to fail if either release binary is missing. A standalone
   `go test` without the gate flag may skip when artifacts are absent; the
   release gate cannot skip the manifest check.
3. Swaps the validated build into `internal/webassets/dist` only after the
   validation passes, then links `CGO_ENABLED=0` binaries for
   `linux/amd64` and `linux/arm64` with the build identity
   (`-ldflags -X` into `internal/buildinfo`: version, short commit, UTC
   build time). The version defaults to `dev` and can be pinned with
   `MEDIA_CORE_VERSION=<version>`.
4. Verifies both artifacts' ELF magic and target machine types with
   `go test ./tests/packaging -run TestPackageReleaseArtifacts`, then
   smoke-tests the host-matching artifact (`version`, `init`, `list` in a
   throwaway state dir). The other architecture is format-checked only: it
   is linked and its ELF machine type is verified, but it is not executed
   when the host cannot run it.
5. Restores the tracked placeholder in `internal/webassets/dist`, so the
   working tree stays clean; the release binaries in `release/` keep the
   real assets.

Artifacts:

```
release/
  media-core-linux-amd64
  media-core-linux-arm64
```

Both are statically linked (`CGO_ENABLED=0`), self-contained, and embed
the validated UI. `media-core version` prints the identity line:
`media-core <version> (commit <commit>, built <time>, go <go>)`.

The embedded UI is the validated build when the script has run and the
tracked placeholder otherwise; the placeholder renders a notice that the
release build has not been staged. Staged build files are git-ignored
(`internal/webassets/dist/*` except the placeholder); `release/` is
git-ignored as well. Nothing in the build publishes: no tag, no push, no
upload.

## Running the service

The state directory holds the database, the owner secret and the
maintenance lock. Initialize it once:

```
media-core init --state-dir <dir>
```

Then serve. The default listener is loopback; there is no wildcard
fallback.

```
media-core serve --state-dir <dir>
```

The UI is served same-origin at `/` (with client-side route fallback),
the API under `/api/v1`, and the owner-secret login at
`POST /api/v1/session`. Static assets under `/assets/` are served
`immutable` (Vite hashes their names); the index shell is served
`no-cache` so a new release is picked up on the next load.

### Remote access

The supported remote topology keeps the listener on loopback and tunnels
through SSH, so the TLS and origin configuration stays out of the
tunnel:

```
ssh -N -L 8480:127.0.0.1:8480 user@host
```

pointed at a local `media-core serve --state-dir <dir> --listen
127.0.0.1:8480`. The browser then talks to `http://127.0.0.1:8480` and
the loopback security policy (host and origin checks) is unchanged.

A directly reachable listener is also possible but requires **all** of:
`--allow-remote`, an https `--public-origin`, and `--tls-cert` /
`--tls-key` files. Missing any of them is a validation error before the
listener binds. The public origin must be https with a host and no
credentials; the origin is the only accepted `Origin` for unsafe
requests, and the session cookie is bound to it.

Trusted LAN is a separate, explicit mode for other devices on a home
network. It is never inferred from a private IP, and it never changes the
remote HTTPS rules above. See [Trusted LAN deployment](#trusted-lan-deployment).

### Owner secret

The owner secret is created only by `media-core owner generate --state-dir
<dir>` and stored in the state directory with mode 0600; neither `init` nor
`serve` creates it, and login fails with a distinct "not configured" 401
until it exists. It never leaves the state directory: backups and exports
package only the database, and restore retains the destination's own
secret file. Regeneration is idempotent and never overwrites an existing
secret, so a forgotten secret is an operator recovery decision, not a
`generate` retry. Sessions are in-memory and do not survive a restart.

## MCP

`media-core mcp --state-dir <dir>` speaks MCP over stdio with the same
state; it starts no listener and needs no Node. Clients connect over a
local pipe or an SSH-forwarded stdio session, which keeps the same
loopback-only posture as the service. `media-core mcp --http` opts into a
loopback Streamable HTTP listener mounted only at `/mcp`; every request
needs the state directory's owner secret as a Bearer token, and a
non-loopback bind requires the same allow-remote + https public-origin +
TLS trio as `serve`. The Origin guard is configured by the SDK; `serve`
never mounts an MCP endpoint.

## Unsupported architectures

The release script always builds both `linux/amd64` and `linux/arm64`,
but it can only execute the binary that matches the host. It invokes the
packaging test for both artifacts, which checks the ELF magic and the
`e_machine` field against the target architecture; the non-host artifact is
then reported as format-checked only. Run it on a matching host (or
emulator) for a full smoke.

## First-run setup (operator runbook)

1. Install the binary and keep state outside the checkout: `/opt/media-core/media-core`, state in `/var/lib/media-core` (directory 0700, files 0600).
2. `media-core init --state-dir /var/lib/media-core` creates and migrates the database. It does **not** create the owner secret.
3. `media-core owner generate --state-dir /var/lib/media-core` writes a 256-bit secret to a 0600 file and prints it once to the local operator's stdout; `owner show` displays it again locally. Never put it in a unit file, workflow, URL or browser storage.
4. Start with `media-core serve --state-dir /var/lib/media-core --listen 127.0.0.1:8787` (or enable the example `deploy/media-core.service`). Log in from the same host or through an SSH port forward; direct remote access additionally requires `--allow-remote`, an https `--public-origin` and a TLS certificate/key.

## Service management

`deploy/media-core.service`, `deploy/media-core-backup.service` and `deploy/media-core-backup.timer` are examples, not an installation step. They use a dedicated `media-core` account, `NoNewPrivileges`, `ProtectSystem=strict`, `RestrictSUIDSGID` and explicit `ReadWritePaths` for the state and backup directories; they contain no secret values. Review paths and the account before enabling any unit.

The backup unit sets `Environment=MEDIA_CORE_BIN=/opt/media-core/media-core`; the smoke script defaults to that installed absolute path and rejects a relative or non-executable override, so a scheduled run does not depend on `PATH`. Before enabling the timer, create `/var/lib/media-core` and `/var/backups/media-core` owned by `media-core` with mode 0700, install the binary at `/opt/media-core/media-core` and the executable script at `/opt/media-core/scripts/backup-smoke.sh`, and verify that the service account can read the working directory and open `<state-dir>/maintenance.lock` for the shared backup lock. `ProtectSystem=strict`, `ProtectHome` and `PrivateTmp` are compatible with the smoke because only the state and backup directories (plus the private temp directory) need writes; a running `serve`/`mcp` process only holds the shared lock and does not block a backup. The timer uses `Persistent=true` and `WantedBy=timers.target`, so a missed run is executed after reboot. Errors exit non-zero, and neither unit nor script log the owner secret. `tests/packaging/deployment_test.go` checks these configuration invariants and runs a systemd-like smoke with an explicit binary path; it does not start real systemd.

## Backup and restore smoke

`scripts/backup-smoke.sh <state-dir> <backup-dir> [prefix]` creates a consistent snapshot archive, restores it into a fresh temporary state directory and runs a read there. The timer passes the prefix `media-core-auto-`; a manual invocation defaults to `media-core-smoke-`. It never stops the service, changes the live database or publishes anything. The destination directory must be outside the state directory. Run it manually after setup and schedule it with `deploy/media-core-backup.timer` (daily by default). A real restore of a live state remains an explicit operator action: stop all handles first, validate the archive, then use `media-core restore <archive> --state-dir <dir> [--force]`.

## Trusted LAN deployment

Trusted LAN serves the unmodified Go binary over plain HTTP on one
explicitly chosen private interface. There is no reverse proxy, Caddy,
Nginx, Docker, second IP per application, built-in DNS server or public
HTTPS. The NAS DNS name is resolved by the router or another local DNS
server that the operator controls; Media Core never changes network or
router configuration.

```
media-core serve \
  --state-dir /var/lib/media-core \
  --trusted-lan \
  --listen <NAS_LAN_IP>:8787 \
  --lan-host media.home.arpa
```

`--trusted-lan` must be explicit. Without it the default stays
`127.0.0.1:8787`, and a non-loopback bind still requires the full remote
HTTPS trio. The listener must be a concrete RFC1918 IPv4 address
(`10/8`, `172.16/12`, `192.168/16`) or an IPv6 ULA in `fc00::/7`.
Wildcards (`0.0.0.0`, `::`), loopback, link-local, multicast, public IPs,
hostnames and an IPv4-mapped IPv6 address are rejected before binding.
`--lan-host` accepts exactly one lowercase DNS name without scheme, port,
path or wildcard; `home.arpa` is the recommended home zone, while `.local`
(mDNS) is rejected. The two accepted `Host` spellings are the listener IP
and the configured name on the same port, and the only accepted unsafe
request `Origin` values are their `http://` forms. Unknown Host, foreign or
missing Origin, missing CSRF token and missing session are all refused;
there is no permissive CORS, no wildcard and no security-header downgrade.

### Security limits (read before enabling)

HTTP is **not** a secure context and does not encrypt anything. The owner
secret sent at login, the session cookie and all library traffic travel in
clear text on the LAN. A malicious device on the same network can sniff or
modify them, and a private IP or a local DNS name does **not** by itself
prove isolation from the internet. Enable Trusted LAN only on a trusted
home network, restrict TCP/8787 with the NAS firewall to known LAN clients,
and keep WAN/UPnP forwarding closed. Never reuse the owner secret on an
untrusted network. The session cookie stays `HttpOnly` and `SameSite=Strict`;
it is deliberately **not** `Secure` on HTTP (a `Secure` cookie would not be
returned and login would fail), while HTTPS remote mode still requires
`Secure`. `crypto.randomUUID` is unavailable on HTTP LAN origins, so the
web client builds UUID v4 request ids from `crypto.getRandomValues`.

### 1. Find and pin the NAS LAN IP

On the NAS:

```bash
ip -4 -o addr show scope global
hostname -I
```

Use the address of the interface that reaches the rest of the home network
(for example `192.168.1.100` in examples below; substitute the real value
everywhere). In the router's DHCP client list, give the NAS a **DHCP
reservation** (static lease) bound to its MAC address, or configure a
static address on the NAS outside the DHCP pool. Without a stable address,
the DNS A record silently points at the wrong host after a lease change.

### 2. Create the local DNS record

In the router's Local DNS / DNS Hostnames / DHCP DNS page, add an A record:

```
media.home.arpa → <NAS_LAN_IP>
```

Use `home.arpa`, not `.local`: `.local` is normally owned by mDNS and a
plain DNS A record there is unreliable. Do not point the record at a public
DNS provider, and do not configure the router to forward this zone to the
internet. If the router cannot hold local records, an operator may install
AdGuard Home or Pi-hole as an **optional** alternative; neither is required
by Media Core.

Several applications can share the NAS IP because each owns its own port:

```
media.home.arpa      A → <NAS_LAN_IP>     http://media.home.arpa:8787
inventory.home.arpa  A → <NAS_LAN_IP>     http://inventory.home.arpa:8080
releases.home.arpa   A → <NAS_LAN_IP>     http://releases.home.arpa:9090
```

ResLedger and Release Watcher above are illustrative only. Each application
listens on its own port and handles its own requests; Media Core neither
proxies nor depends on them.

### 3. Why the port stays in the URL

DNS maps a name to an IP address only; it does not advertise TCP ports and
browser HTTP requests do not use DNS SRV records for ordinary web URLs.
Therefore the URL must spell the port: `http://media.home.arpa:8787`.
The hostname and the IP both work, both with `:8787`, and only those two
spellings are accepted by the server.

### 4. Verify DNS from each client

```bash
getent hosts media.home.arpa
nslookup media.home.arpa      # Windows / macOS alternative
```

The result must be `<NAS_LAN_IP>`. On Windows PowerShell use
`Resolve-DnsName media.home.arpa`.

### 5. Open the UI

- Linux: `xdg-open http://media.home.arpa:8787`
- macOS: `open http://media.home.arpa:8787`
- Windows: open the URL in Edge/Chrome; `start http://media.home.arpa:8787` also works.

On a phone, use the browser address bar on the same Wi-Fi:
`http://media.home.arpa:8787`. Sign in with the owner secret generated by
`media-core owner generate --state-dir /var/lib/media-core`.

If a device uses an external DNS resolver, VPN or Android/iOS Private DNS
(DoT/DoH), its resolver may never see the router's local zone. Point that
device at the router as its DNS server, add the zone/record to the DNS
server it actually uses, or exclude `home.arpa` from private/secure DNS
forwarding. A working IP URL with a failing hostname URL means DNS, not
Media Core, is the problem.

### 6. Verify firewall and WAN exposure

From another LAN device:

```bash
curl -I http://media.home.arpa:8787/
nc -vz <NAS_LAN_IP> 8787
```

On a network outside the home, `curl --connect-timeout 5
http://<NAS_PUBLIC_IP>:8787/` must fail (timeout/connection refused).
Check the router for WAN port forwards and UPnP rules covering 8787, and
the NAS firewall (for example `ufw status verbose`) for an explicit allow
from the LAN subnet only. Do not treat RFC1918 addressing as a firewall.

### 7. Verify Origin, HTTP and proxy assumptions

The browser must request exactly `http://media.home.arpa:8787` (or the
exact IP). A page opened from `http://some-other-name:8787` is rejected as
an invalid Host, and a form posted from a different origin is rejected as
an invalid Origin even if the URL is correct. There is no HTTPS listener in
Trusted LAN mode; use an SSH tunnel or the authenticated TLS remote trio if
encryption is required. Requests go straight to the Go listener: a browser
DevTools network entry should show `http://media.home.arpa:8787/api/v1/...`
with no proxy hop.

### 8. Systemd drop-in (Ubuntu NAS)

Keep `deploy/media-core.service` unchanged; install the example drop-in
after replacing the placeholder IP and hostname:

```bash
sudo install -d -m 0755 /etc/systemd/system/media-core.service.d
sudo install -m 0644 deploy/media-core.service.d/trusted-lan.conf \
  /etc/systemd/system/media-core.service.d/trusted-lan.conf
sudo editor /etc/systemd/system/media-core.service.d/trusted-lan.conf
sudo systemd-analyze verify /etc/systemd/system/media-core.service
sudo systemctl daemon-reload
sudo systemctl restart media-core
systemctl --no-pager status media-core
```

The drop-in clears the default `ExecStart`, sets the Trusted LAN command,
waits for `network-online.target`, and inherits the dedicated user,
`NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths`, restart policy
and backup timer from the base units. It contains no secrets. Remove the
drop-in and reload to return to the loopback default.

### 9. Install commands on an Ubuntu NAS

```bash
sudo useradd --system --home-dir /nonexistent --shell /usr/sbin/nologin media-core
sudo install -d -o media-core -g media-core -m 0700 /var/lib/media-core /var/backups/media-core
sudo install -d -o media-core -g media-core -m 0755 /opt/media-core /opt/media-core/scripts
sudo install -o media-core -g media-core -m 0755 release/media-core-linux-amd64 /opt/media-core/media-core
sudo install -o media-core -g media-core -m 0755 scripts/backup-smoke.sh /opt/media-core/scripts/backup-smoke.sh
/opt/media-core/media-core init --state-dir /var/lib/media-core
/opt/media-core/media-core owner generate --state-dir /var/lib/media-core
sudo install -m 0644 deploy/media-core.service /etc/systemd/system/media-core.service
sudo install -m 0644 deploy/media-core-backup.service /etc/systemd/system/media-core-backup.service
sudo install -m 0644 deploy/media-core-backup.timer /etc/systemd/system/media-core-backup.timer
```

Use the `linux-arm64` artifact on ARM NAS hardware. Then follow the drop-in
steps above and enable `media-core.service` and `media-core-backup.timer`
only after reviewing paths and the firewall. No unit file contains the owner
secret; keep the printed secret in the operator's password manager.

## CI and release gate

`.github/workflows/ci.yml` runs build, vet, tests, race tests, shell syntax, structural checks, TypeScript, Vitest, Playwright and a local release build on every push/PR. `.github/workflows/release-gate.yml` is separate and non-publishing: it re-runs verification and uploads local artifacts only. Neither workflow deploys to a NAS.

## Retention

Automatic archives (`media-core-auto-<UTC timestamp>`) receive a `.verified` marker only after a new archive passes restore smoke. `scripts/backup_retention.py <backup-dir>` keeps the newest 7 UTC daily, 4 ISO-weekly and 3 monthly buckets, always keeps the newest verified archive, and deletes only automatic, marked archives. Manual archives, unknown names and symlinks are never touched; deletions run under a bounded lock and only after the successful smoke that invoked the script. Use `--dry-run` to print the exact deletion plan without changing files. Retention is not a substitute for a separate copy on another physical medium.
