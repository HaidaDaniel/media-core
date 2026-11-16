#!/usr/bin/env bash
# Create one consistent backup of a Media Core state directory and prove it
# can be restored into a fresh scratch state directory. The live database is
# never changed, and no service needs to be stopped: Create uses VACUUM INTO
# under the shared maintenance lock, while Restore only touches the scratch
# destination. Nothing here deploys or publishes anything.
set -euo pipefail

STATE_DIR=${1:?usage: backup-smoke.sh <state-dir> <dest-dir>}
DEST_DIR=${2:?usage: backup-smoke.sh <state-dir> <dest-dir>}
PREFIX=${3:-media-core-smoke-}
BIN=${MEDIA_CORE_BIN:-/opt/media-core/media-core}
SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)

case "$PREFIX" in
  *[!A-Za-z0-9-]*) echo "error: unsafe archive prefix: $PREFIX" >&2; exit 2 ;;
esac

if [[ "$BIN" != /* ]]; then
  echo "error: MEDIA_CORE_BIN must be an absolute path, got: $BIN" >&2
  exit 2
fi
if [ ! -f "$BIN" ] || [ ! -x "$BIN" ]; then
  echo "error: media-core binary is missing or not executable: $BIN" >&2
  exit 2
fi

for path in "$STATE_DIR" "$DEST_DIR"; do
  if [ ! -d "$path" ]; then
    echo "error: directory does not exist: $path" >&2
    exit 1
  fi
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

NAME="${PREFIX}$(date -u +%Y%m%dT%H%M%SZ)"
ENVELOPE=$("$BIN" backup create --state-dir "$STATE_DIR" --dest "$DEST_DIR" --name "$NAME")
ARCHIVE=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["archive"])' <<<"$ENVELOPE")
if [ ! -d "$ARCHIVE" ]; then
  echo "error: backup did not publish an archive directory: $ARCHIVE" >&2
  exit 1
fi

RESTORE_DIR="$TMP/restored"
"$BIN" restore "$ARCHIVE" --state-dir "$RESTORE_DIR" --force >/dev/null
"$BIN" list --state-dir "$RESTORE_DIR" >/dev/null
MARKER="$ARCHIVE.verified"
printf 'verified %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" > "$MARKER"
chmod 0600 "$MARKER"
if [ "$PREFIX" = "media-core-auto-" ]; then
  python3 "$SCRIPT_DIR/backup_retention.py" "$DEST_DIR" >/dev/null
fi
echo "backup smoke ok: $ARCHIVE"
