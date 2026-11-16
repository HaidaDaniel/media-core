#!/usr/bin/env bash
# Create one consistent backup of a Media Core state directory and prove it
# can be restored into a fresh scratch state directory. The live database is
# never changed, and no service needs to be stopped: Create uses VACUUM INTO
# under the shared maintenance lock, while Restore only touches the scratch
# destination. Nothing here deploys or publishes anything.
set -euo pipefail

STATE_DIR=${1:?usage: backup-smoke.sh <state-dir> <dest-dir>}
DEST_DIR=${2:?usage: backup-smoke.sh <state-dir> <dest-dir>}
BIN=${MEDIA_CORE_BIN:-media-core}

for path in "$STATE_DIR" "$DEST_DIR"; do
  if [ ! -d "$path" ]; then
    echo "error: directory does not exist: $path" >&2
    exit 1
  fi
done

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

NAME="media-core-smoke-$(date -u +%Y%m%dT%H%M%SZ)"
ENVELOPE=$("$BIN" backup create --state-dir "$STATE_DIR" --dest "$DEST_DIR" --name "$NAME")
ARCHIVE=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["archive"])' <<<"$ENVELOPE")
if [ ! -d "$ARCHIVE" ]; then
  echo "error: backup did not publish an archive directory: $ARCHIVE" >&2
  exit 1
fi

RESTORE_DIR="$TMP/restored"
"$BIN" restore "$ARCHIVE" --state-dir "$RESTORE_DIR" --force >/dev/null
"$BIN" list --state-dir "$RESTORE_DIR" >/dev/null
echo "backup smoke ok: $ARCHIVE"
