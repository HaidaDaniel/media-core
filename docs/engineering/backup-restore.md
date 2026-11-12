# Backup and restore

This document freezes the snapshot archive contract: task 0022 implements
the backup side and task 0023 the restore side, both against this
contract.

## Archive layout

An archive is one directory, published atomically, containing exactly two
files:

```
<name>/
  media-core.db    the consistent snapshot database (mode 0600)
  manifest.json    the integrity metadata (mode 0600)
```

The directory itself uses mode 0700. The name is a single path element;
without an explicit name the command supplies
`media-core-snapshot-<UTC timestamp>`.

## Manifest

The manifest is JSON with the marker `format: media-core-snapshot-v1` and
the fields:

| Field | Meaning |
| --- | --- |
| `created_at` | When the archive was published (UTC). |
| `app` | Build identity (version, commit, build time, Go version) of the binary that created the archive. |
| `schema` | Schema ledger version of the snapshot. |
| `db.name` | Fixed `media-core.db`. |
| `db.size` | Snapshot file size in bytes. |
| `db.sha256` | SHA-256 of the snapshot file. |

## Snapshot semantics

- The snapshot is taken with `VACUUM INTO` while holding the **shared**
  maintenance lock, so normal writers may keep running. The snapshot
  contains every commit made before the `VACUUM INTO` finished, including
  commits that are still in the WAL; it is a consistent committed state,
  never a bare copy of the live database file.
- The snapshot boundary is the finish of the `VACUUM INTO`. The manifest
  never claims anything about commits made after that boundary; a later
  writer's work is simply not in the archive.
- The source database is opened read-only for the backup and is never
  modified by it. A source at a missing, pending or newer schema is
  rejected with the schema boundary errors; a missing source database, a
  symlinked source file, or a state directory locked by an exclusive
  holder (init/migrate/restore) fails before any archive work starts.

## Publishing and overwrite

- The destination must not be the state directory or below it: an archive
  is never published into the directory it backs up, so an overwrite can
  never reach a state file. The destination is resolved through symlinks
  before the check, so a link cannot redirect the publish into the state
  directory.
- The archive is staged in a hidden temporary directory inside the
  destination and published with a single rename: the archive appears as a
  complete directory or not at all. Every failure path removes the staging
  directory and restores any set-aside archive, so no apparently complete
  corrupt archive and no staging debris remain, and the source is
  untouched.
- An existing target archive is never replaced unless the caller passes an
  explicit overwrite; the refusal is reported as a conflict and leaves the
  existing archive byte-identical. Overwrite sets the replaced archive
  aside under a sibling `<name>.replaced-<timestamp>` name before
  publishing the new one, and the set-aside copy is removed only after the
  new archive is fully in place, so a crash at any point loses neither
  archive; a stale set-aside name, if ever left behind, is a complete
  prior archive.
  A publish that loses the race to a concurrent publisher is reported as a
  conflict.

## Exclusions

The archive packages only the snapshot database and the manifest. Nothing
else from the state directory is ever copied, so owner secrets, session
material and TLS keys are excluded by construction; the manifest contains
no state directory contents beyond the measured database file metadata.

## Restore contract

Restore (task 0023) accepts an archive only when, in order: the archive
path is a directory (not a symlink) holding exactly the two regular files
`media-core.db` and `manifest.json`; the manifest parses, carries the
format `media-core-snapshot-v1`, names the database file, and holds a
positive schema; the packaged file's size and SHA-256 match the manifest;
`PRAGMA integrity_check` on the file returns ok; the file's schema ledger
matches the manifest's schema; and the ledger sits exactly at the
restoring build's current schema — an older or newer backup is rejected
with the distinct schema boundary error. All of this happens before the
destination is touched.

The replacement then runs under the **exclusive** lifetime maintenance
lock, so any active shared-lock holder — a running serve, MCP or CLI
handle — blocks it with the locked error. A destination that already
contains a database requires an explicit force; without it the restore
fails as a conflict and leaves the destination byte-identical. The
validated snapshot is staged in a temporary file inside the destination
state directory (same filesystem) and replaces the database file with one
atomic rename, after the old database's WAL sidecars are removed. The
destination state directory is never cleared: only the database file and
its sidecars are replaced, so an owner secret or any other state file is
retained. A failure at any point — validation, staging, locking or
publishing — leaves the previous database usable with a distinguishable
error and no staging debris.
