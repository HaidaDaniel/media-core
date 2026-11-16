#!/usr/bin/env bash
# build-release.sh builds the local release artifacts for media-core:
# the frontend build staged into the Go embed, and CGO-disabled Linux
# amd64/arm64 binaries with truthful link-time identity. It builds and
# tests local artifacts only: it never publishes, tags, pushes or uploads
# anything, and it never touches a state directory outside its own temp
# dirs.
set -euo pipefail
cd "$(dirname "$0")/.."

STAGE=internal/webassets/dist
WEB_DIST=web/dist
OUT=release
PKG=github.com/HaidaDaniel/media-core/internal/buildinfo

echo "== media-core release build (local artifacts only; nothing is published)"

# -- 1. Frontend build: the only Node-dependent step. -------------------
if ! command -v pnpm >/dev/null 2>&1; then
  echo "error: pnpm is required to build the frontend (Node is never needed at runtime)" >&2
  exit 1
fi
echo "-- building the frontend (web/)"
pnpm --dir web exec vite build

# -- 2. Stage the build into the embed and validate the manifest. -------
echo "-- staging $WEB_DIST -> $STAGE"
if [ ! -f "$STAGE/index.html" ]; then
  echo "error: the tracked placeholder $STAGE/index.html is missing; restore it from git" >&2
  exit 1
fi
# Keep the placeholder so the trap can restore it without git, and stage
# in a temp dir so a failed validation never destroys the tracked shell.
PLACEHOLDER_TMP=$(mktemp)
MANIFEST_TMP=$(mktemp)
STAGING_TMP=$(mktemp -d)
PLACEHOLDER_MODE=$(stat -c %a "$STAGE/index.html")
restore_placeholder() {
  if [ -s "$PLACEHOLDER_TMP" ]; then
    rm -rf "$STAGE"
    mkdir -p "$STAGE"
    cp "$PLACEHOLDER_TMP" "$STAGE/index.html"
    chmod "$PLACEHOLDER_MODE" "$STAGE/index.html"
  fi
}
cleanup() {
  restore_placeholder
  rm -f "$PLACEHOLDER_TMP" 2>/dev/null || true
  rm -f "${MANIFEST_TMP:-}" 2>/dev/null || true
  if [ -n "${STAGING_TMP:-}" ]; then rm -rf "$STAGING_TMP" 2>/dev/null || true; fi
}
# The trap restores the placeholder on every exit path, so a failure after
# the staging swap never leaves the tracked shell replaced or its mode
# altered; the success path calls it again for the message.
trap cleanup EXIT
cp "$STAGE/index.html" "$PLACEHOLDER_TMP"
cp -r "$WEB_DIST/." "$STAGING_TMP/"

# Validation: a shell that exists and is non-empty, no external
# references, and every local reference resolving inside the staging.
if [ ! -s "$STAGING_TMP/index.html" ]; then
  echo "error: staged index.html is missing or empty" >&2
  exit 1
fi
if grep -qE '(src|href)="https?://' "$STAGING_TMP/index.html"; then
  echo "error: the shell references external http(s) URLs; the release build must be self-contained" >&2
  exit 1
fi
while IFS= read -r ref; do
  case "$ref" in
    http://*|https://*|data:*|mailto:*|\#*|"") continue ;;
  esac
  rel="${ref#?}"
  if [ ! -f "$STAGING_TMP/$rel" ]; then
    echo "error: the shell references $ref, which is not in the staged build" >&2
    exit 1
  fi
done < <(grep -oE '(src|href)="[^"]+"' "$STAGING_TMP/index.html" | sed 's/^.[^=]*="//; s/"$//')

# Manifest: every staged file with its size and SHA-256, in stable order.
{
  printf '{\n  "format": "media-core-webassets-v1",\n  "files": [\n'
  first=1
  while IFS= read -r f; do
    size=$(wc -c < "$STAGING_TMP/$f" | tr -d ' ')
    sum=$(sha256sum "$STAGING_TMP/$f" | cut -d' ' -f1)
    if [ "$first" -eq 1 ]; then first=0; else printf ',\n'; fi
    printf '    {"name": "%s", "size": %s, "sha256": "%s"}' "$f" "$size" "$sum"
  done < <(cd "$STAGING_TMP" && find . -type f | sed 's|^\./||' | LC_ALL=C sort)
  printf '\n  ]\n}\n'
} > "$MANIFEST_TMP"
mv "$MANIFEST_TMP" "$STAGING_TMP/manifest.json"

# Everything validated: swap the staging into the embed.
rm -rf "$STAGE"
mv "$STAGING_TMP" "$STAGE"
STAGING_TMP=""
echo "-- staged $(find "$STAGE" -type f | wc -l) files; asset manifest validated"

# -- 3. Truthful identity and the cross builds. --------------------------
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILDTIME=$(date -u +%Y-%m-%dT%H:%M:%SZ)
VERSION="${MEDIA_CORE_VERSION:-dev}"
LDFLAGS="-X ${PKG}.Version=${VERSION} -X ${PKG}.Commit=${COMMIT} -X ${PKG}.BuildTime=${BUILDTIME}"
echo "-- identity: version=${VERSION} commit=${COMMIT} built=${BUILDTIME}"

mkdir -p "$OUT"
echo "-- linking linux/amd64"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$LDFLAGS" -o "$OUT/media-core-linux-amd64" ./cmd/media-core
echo "-- linking linux/arm64"
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$LDFLAGS" -o "$OUT/media-core-linux-arm64" ./cmd/media-core
echo "-- format check: both artifacts are the expected ELF machine type"
go test ./tests/packaging -run TestPackageReleaseArtifacts

# -- 4. Smoke the host-matching artifact; report the limits for the rest.
HOST_ARCH=$(uname -m)
HOST_BIN=""
case "$HOST_ARCH" in
  x86_64) HOST_BIN="$OUT/media-core-linux-amd64" ;;
  aarch64) HOST_BIN="$OUT/media-core-linux-arm64" ;;
esac
if [ -n "$HOST_BIN" ]; then
  echo "-- smoke: $(basename "$HOST_BIN")"
  "$HOST_BIN" version
  TMPSTATE=$(mktemp -d)
  "$HOST_BIN" init --state-dir "$TMPSTATE"
  "$HOST_BIN" list --state-dir "$TMPSTATE" >/dev/null
  rm -rf "$TMPSTATE"
  OTHER="$OUT/media-core-linux-arm64"
  [ "$HOST_ARCH" = "aarch64" ] && OTHER="$OUT/media-core-linux-amd64"
  echo "-- smoke limit: $(basename "$OTHER") is not executable on this host (${HOST_ARCH}); it is format-checked only"
else
  echo "-- smoke limit: host arch ${HOST_ARCH} is not a release target; both artifacts are format-checked only"
fi

# -- 5. Restore the tracked placeholder so the tree stays clean; the
#    linked binaries in release/ keep the real assets. The EXIT trap does
#    the same on any earlier failure.
restore_placeholder
echo "-- restored the tracked placeholder in $STAGE (release binaries keep the staged build)"

echo "== artifacts (sha256):"
sha256sum "$OUT/media-core-linux-amd64" "$OUT/media-core-linux-arm64"
echo "== done; local artifacts only, nothing published"
