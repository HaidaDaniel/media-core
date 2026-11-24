# Backup and restore

This document freezes the snapshot archive contract: task 0022 implements
the backup side and task 0023 the restore side, both against this
contract.

## Archive layout

An archive is one directory, published atomically:

```
<name>/
  media-core.db        the consistent snapshot database (mode 0600)
  manifest.json        the integrity metadata (mode 0600)
  artwork-cache/       selected cover bytes referenced by active rows (mode 0700)
```

The `artwork-cache/` directory is optional: it is present only when at least
one active selection has cached bytes. Archives written before task 0080
(`media-core-snapshot-v1`) contain only the two files and remain restorable.

The directory itself uses mode 0700. The name is a single path element;
without an explicit name the command supplies
`media-core-snapshot-<UTC timestamp>`.

## Manifest

The manifest is JSON with the marker `format: media-core-snapshot-v2` and
the fields:

| Field | Meaning |
| --- | --- |
| `created_at` | When the archive was published (UTC). |
| `app` | Build identity (version, commit, build time, Go version) of the binary that created the archive. |
| `schema` | Schema ledger version of the snapshot. |
| `db.name` | Fixed `media-core.db`. |
| `db.size` | Snapshot file size in bytes. |
| `db.sha256` | SHA-256 of the snapshot file. |
| `artwork.files` | Selected cover files with `name`, `size` and `sha256`. |
| `artwork.missing` | Referenced cache names that were already absent at backup time; recorded, never invented. |

## Snapshot semantics

- Concurrent publishers of the same archive name serialize on a `.media-core-archive.lock` file in the destination directory; it is an operational lock, not an archive.
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

## What is included and excluded

The archive packages the snapshot database, the manifest and only the
`artwork-cache/` files referenced by active `work_artwork` rows. Orphaned
previews, owner secrets, session material and TLS keys never enter the
archive. The selected cover bytes are user-visible state, not disposable
temporary data, so they travel with the snapshot; the hard 64 MiB cache cap
bounds the archive growth. If a referenced cache file is already missing at
backup time, the manifest records it under `artwork.missing` and the
database row still travels; the library shows a neutral placeholder after
restore until the user explicitly selects a cover again.

## Restore contract

Restore (task 0023) accepts an archive only when, in order: the archive
path is a directory (not a symlink) holding `media-core.db`, `manifest.json`
and, for v2, an optional `artwork-cache/` directory of regular files; the
manifest parses, carries `media-core-snapshot-v1` or
`media-core-snapshot-v2`, names the database file, and holds a positive
schema; the packaged database and every listed artwork file match the
manifest's size and SHA-256; `PRAGMA integrity_check` on the database
returns ok; the schema ledger matches and sits exactly at the restoring
build's current schema — an older or newer backup is rejected with the
distinct schema boundary error. v2 rejects extra artwork entries, symlinks
and a manifest/list mismatch. All of this happens before the destination is
touched.

The replacement then runs under the **exclusive** lifetime maintenance
lock, so any active shared-lock holder — a running serve, MCP or CLI
handle — blocks it with the locked error. A destination that already
contains a database requires an explicit force; without it the restore
fails as a conflict and leaves the destination byte-identical. The
validated snapshot is staged in a temporary file inside the destination
state directory (same filesystem) and replaces the database file with one
atomic rename. Before the old database's WAL sidecars are removed, restore
checkpoints the destination under the exclusive maintenance lock so an
acknowledged transaction that a crashed writer left only in the WAL becomes
part of the main file; the staged file is synced before the rename, and the rename is followed by a directory sync. The
 destination state directory is never cleared: only the database file and
 its sidecars are replaced, so an owner secret or any other state file is
 retained. A failure before publication — validation, staging, locking,
 checkpoint or rename — leaves the previous database usable with a
 distinguishable error and no staging debris. A directory-sync failure is
 reported after publication, when the restored snapshot is already in place.

## Complete data coverage (task 0047)

Scenario F (`tests/acceptance/backup_test.go`) exercises the contract
against the complete M5 dataset, not a subset: works with aliases,
editions, consumption, intention, progress with an explicit correction,
notes with a superseded revision, ratings, references, a confirmed and a
proposed relation, a confirmed origin, and the import/binding history. It
verifies, in order:

- a consistent backup taken while a second process keeps committing
  (the snapshot provably contains post-fixture commits and the snapshot
  boundary is respected);
- the archive holds exactly the database and the manifest for a fixture
  without artwork — no owner secret, no state file; the separate artwork
  backup test proves selected cover bytes travel in v2 and restore exactly;
- each invalid archive — a symlinked (traversal) database entry, a
  corrupted hash, a size-mismatching manifest, and a newer schema — is
  rejected with the right error **before** the destination is touched,
  leaving the destination byte-identical;
- the restore into a fresh isolated state directory retains the
  destination's own owner secret byte-identically with its 0600 mode,
  the state directory at 0700 and the database at 0600;
- the restored state re-asserts the complete fixture through the shared
  service, including retained history rows and their correction labels;
- the Web API and MCP report the same normalized state (ids and instants
  stripped) for the works list, a work detail, an edition detail, an
  edition history, work relations, and the activity feed;
- an active handle blocks the replacement with the locked error, and a
  refused overwrite leaves the destination byte-identical.

The readable export carries the same complete dataset: the export
registry is required to cover every live table in the schema ledger, and
`internal/app` asserts that invariant (`TestExportRegistryCoversSchema`).

## Retention of automatic archives

The timer-created prefix `media-core-auto-` and a `.verified` marker identify archives eligible for retention. `scripts/backup_retention.py` keeps the newest 7 daily, 4 weekly and 3 monthly UTC buckets, preserves the newest verified archive, and never deletes manual or unknown archives or symlinks. It runs only after a successful backup-and-restore smoke, under a bounded lock, and supports `--dry-run`. Retention bounds local archive growth only; it does not replace an independent copy on another physical medium.
