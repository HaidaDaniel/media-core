#!/usr/bin/env python3
"""Safe retention for automatic Media Core backup archives.

Only archives named by the automatic backup workflow and carrying a verified
marker are candidates. Manual archives, unknown names and symlinks are never
considered. Deletions happen under a bounded lock and only after a newer
archive has passed restore smoke.
"""
import argparse
import datetime as dt
import fcntl
import os
import re
import stat
import sys
import time

DEFAULT_PREFIX = "media-core-auto-"
MARKER_SUFFIX = ".verified"
ARTWORK_DIR = "artwork-cache"
LOCK_NAME = "media-core-retention.lock"


def parse_timestamp(name, prefix=DEFAULT_PREFIX):
    """Return the UTC timestamp encoded in an automatic archive name."""
    pattern = re.compile(r"^" + re.escape(prefix) + r"(\d{8}T\d{6}Z)$")
    match = pattern.match(name)
    if not match:
        return None
    try:
        return dt.datetime.strptime(match.group(1), "%Y%m%dT%H%M%SZ").replace(tzinfo=dt.timezone.utc)
    except ValueError:
        return None


def buckets(timestamp):
    """UTC day, ISO week and month buckets for one timestamp."""
    iso = timestamp.isocalendar()
    return (
        timestamp.date().isoformat(),
        f"{iso.year}-W{iso.week:02d}",
        timestamp.strftime("%Y-%m"),
    )


def verified_archives(directory, prefix=DEFAULT_PREFIX):
    """List (timestamp, name) of automatic, verified, real archive directories."""
    found = []
    with os.scandir(directory) as entries:
        for entry in entries:
            timestamp = parse_timestamp(entry.name, prefix)
            if timestamp is None:
                continue
            info = entry.stat(follow_symlinks=False)
            if not stat.S_ISDIR(info.st_mode):
                continue
            marker = os.path.join(directory, entry.name + MARKER_SUFFIX)
            try:
                marker_info = os.lstat(marker)
            except FileNotFoundError:
                continue
            if not stat.S_ISREG(marker_info.st_mode):
                continue
            found.append((timestamp, entry.name))
    found.sort(key=lambda pair: (pair[0], pair[1]), reverse=True)
    return found


def plan_deletions(directory, prefix=DEFAULT_PREFIX, keep_daily=7, keep_weekly=4, keep_monthly=3):
    """Return archive names to delete under the 7/4/3 UTC bucket policy."""
    archives = verified_archives(directory, prefix)
    keep = set()
    seen = {"day": set(), "week": set(), "month": set()}
    limits = (("day", keep_daily), ("week", keep_weekly), ("month", keep_monthly))
    for timestamp, name in archives:
        values = buckets(timestamp)
        for (kind, limit), value in zip(limits, values):
            if len(seen[kind]) < limit and value not in seen[kind]:
                seen[kind].add(value)
                keep.add(name)
    if archives:
        keep.add(archives[0][1])
    return [name for _, name in archives if name not in keep]


def _archive_files(path):
    """Return validated regular files and the v2 artwork-cache directory."""
    with os.scandir(path) as entries:
        children = list(entries)
    top_files = []
    artwork = None
    for child in children:
        child_info = child.stat(follow_symlinks=False)
        if stat.S_ISREG(child_info.st_mode):
            top_files.append(child)
            continue
        if child.name == ARTWORK_DIR and stat.S_ISDIR(child_info.st_mode) and artwork is None:
            artwork = child
            continue
        raise RuntimeError(f"unsafe archive entry: {child.name}")
    nested_files = []
    if artwork is not None:
        with os.scandir(artwork.path) as entries:
            for child in entries:
                child_info = child.stat(follow_symlinks=False)
                if not stat.S_ISREG(child_info.st_mode):
                    raise RuntimeError(f"unsafe archive entry: {ARTWORK_DIR}/{child.name}")
                nested_files.append(child)
    return top_files, artwork, nested_files


def safe_remove_archive(directory, name):
    """Remove one archive and its marker without following symlinks."""
    path = os.path.join(directory, name)
    info = os.lstat(path)
    if not stat.S_ISDIR(info.st_mode):
        raise RuntimeError(f"not a real directory: {name}")
    top_files, artwork, nested_files = _archive_files(path)
    for child in top_files:
        os.remove(child.path)
    if artwork is not None:
        for child in nested_files:
            os.remove(child.path)
        os.rmdir(artwork.path)
    os.rmdir(path)
    marker = os.path.join(directory, name + MARKER_SUFFIX)
    marker_info = os.lstat(marker)
    if stat.S_ISREG(marker_info.st_mode):
        os.remove(marker)


def acquire_lock(directory, timeout):
    """Hold an exclusive advisory lock in the backup directory."""
    path = os.path.join(directory, LOCK_NAME)
    handle = open(path, "a+")
    deadline = time.monotonic() + timeout
    while True:
        try:
            fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            return handle
        except BlockingIOError:
            if time.monotonic() >= deadline:
                handle.close()
                raise RuntimeError("retention lock is busy")
            time.sleep(0.02)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("directory", help="backup archive directory")
    parser.add_argument("--prefix", default=DEFAULT_PREFIX)
    parser.add_argument("--keep-daily", type=int, default=7)
    parser.add_argument("--keep-weekly", type=int, default=4)
    parser.add_argument("--keep-monthly", type=int, default=3)
    parser.add_argument("--lock-timeout", type=float, default=5.0)
    parser.add_argument("--dry-run", action="store_true")
    args = parser.parse_args(argv)
    if not os.path.isdir(args.directory):
        print(f"error: backup directory does not exist: {args.directory}", file=sys.stderr)
        return 1
    try:
        lock = acquire_lock(args.directory, args.lock_timeout)
    except (OSError, RuntimeError) as err:
        print(f"error: cannot lock retention: {err}", file=sys.stderr)
        return 1
    try:
        deletions = plan_deletions(
            args.directory, args.prefix, args.keep_daily, args.keep_weekly, args.keep_monthly
        )
        for name in deletions:
            if args.dry_run:
                print(f"would delete: {name}")
                continue
            try:
                safe_remove_archive(args.directory, name)
            except (OSError, RuntimeError) as err:
                print(f"error: cannot delete {name}: {err}", file=sys.stderr)
                return 1
            print(f"deleted: {name}")
        return 0
    finally:
        fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
        lock.close()


if __name__ == "__main__":
    sys.exit(main())
