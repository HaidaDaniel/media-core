# Backup and restore

This document freezes the snapshot archive contract. Task 0022 implements
the backup side; the restore side is implemented by task 0023 against this
same contract.

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

- The archive is staged in a hidden temporary directory inside the
  destination and published with a single rename: the archive appears as a
  complete directory or not at all. Every failure path removes the staging
  directory, so no apparently complete corrupt archive and no staging
  debris remain, and the source is untouched.
- An existing target archive is never replaced unless the caller passes an
  explicit overwrite; the refusal is reported as a conflict and leaves the
  existing archive byte-identical.

## Exclusions

The archive packages only the snapshot database and the manifest. Nothing
else from the state directory is ever copied, so owner secrets, session
material and TLS keys are excluded by construction; the manifest contains
no state directory contents beyond the measured database file metadata.

## Restore contract

Restore (task 0023) runs under the **exclusive** maintenance lock and
accepts an archive only when: the layout is exactly the two files; the
manifest format is `media-core-snapshot-v1`; `db.size` and `db.sha256`
match the packaged file; `PRAGMA integrity_check` on the file returns ok;
and the schema ledger version is acceptable to the restoring build. The
restored database replaces the state database atomically; a failed
restore leaves the prior state intact with a distinguishable error.
