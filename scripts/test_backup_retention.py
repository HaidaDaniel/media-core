"""Unit tests for the safe backup retention planner."""
import contextlib
import datetime as dt
import io
import os
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import backup_retention as retention


def stamp(day, hour=0, minute=0, second=0):
    return f"{day.replace('-', '')}T{hour:02d}{minute:02d}{second:02d}Z"


def make_archive(directory, name, verified=True, marker_symlink=False):
    path = os.path.join(directory, name)
    os.mkdir(path)
    with open(os.path.join(path, "manifest.json"), "w", encoding="utf-8") as handle:
        handle.write("{}")
    marker = path + retention.MARKER_SUFFIX
    if marker_symlink:
        os.symlink(path, marker)
    elif verified:
        with open(marker, "w", encoding="utf-8") as handle:
            handle.write("verified\n")
    return path


class RetentionTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.dir = self.temp.name

    def test_daily_limit_keeps_newest_seven(self):
        for day in (f"2026-01-{n:02d}" for n in range(1, 11)):
            make_archive(self.dir, retention.DEFAULT_PREFIX + stamp(day))
        delete = retention.plan_deletions(self.dir)
        self.assertEqual(len(delete), 3)
        for name in delete:
            self.assertIn("2026010", name)

    def test_weekly_and_monthly_buckets_overlap(self):
        start_day = dt.date(2026, 1, 1)
        for offset in range(40):
            day = (start_day + dt.timedelta(days=offset)).isoformat()
            make_archive(self.dir, retention.DEFAULT_PREFIX + stamp(day))
        delete = retention.plan_deletions(self.dir)
        names = [name for name in os.listdir(self.dir) if name.startswith(retention.DEFAULT_PREFIX) and name.endswith("Z")]
        kept = [name for name in names if name not in delete]
        newest = retention.DEFAULT_PREFIX + stamp((start_day + dt.timedelta(days=39)).isoformat())
        self.assertIn(newest, kept)
        self.assertGreater(len(delete), 0)
        self.assertLessEqual(len(kept), 7 + 4 + 3)
        self.assertTrue(all(name < newest for name in delete))

    def test_manual_and_unknown_archives_are_never_deleted(self):
        make_archive(self.dir, "manual-" + stamp("2020-01-01"))
        make_archive(self.dir, "media-core-smoke-" + stamp("2020-01-01"))
        make_archive(self.dir, retention.DEFAULT_PREFIX + stamp("2026-01-01"))
        for name in range(8, 15):
            make_archive(self.dir, retention.DEFAULT_PREFIX + stamp(f"2026-01-{name:02d}"))
        delete = retention.plan_deletions(self.dir)
        self.assertTrue(all(name.startswith(retention.DEFAULT_PREFIX) for name in delete))
        self.assertTrue(os.path.isdir(os.path.join(self.dir, "manual-" + stamp("2020-01-01"))))

    def test_unverified_and_symlink_archives_are_ignored(self):
        make_archive(self.dir, retention.DEFAULT_PREFIX + stamp("2020-01-01"), verified=False)
        target = make_archive(self.dir, retention.DEFAULT_PREFIX + stamp("2026-01-01"))
        link = os.path.join(self.dir, retention.DEFAULT_PREFIX + stamp("2020-02-01"))
        os.symlink(target, link)
        os.symlink(target, link + retention.MARKER_SUFFIX)
        self.assertEqual(retention.plan_deletions(self.dir), [])
        with self.assertRaises(RuntimeError):
            retention.safe_remove_archive(self.dir, os.path.basename(link))

    def test_last_verified_archive_is_always_kept(self):
        for day in ("2026-01-01", "2026-01-02"):
            make_archive(self.dir, retention.DEFAULT_PREFIX + stamp(day))
        newest = retention.DEFAULT_PREFIX + stamp("2026-01-02")
        delete = retention.plan_deletions(self.dir, keep_daily=0, keep_weekly=0, keep_monthly=0)
        self.assertEqual(delete, [retention.DEFAULT_PREFIX + stamp("2026-01-01")])

    def test_dry_run_then_apply_and_idempotence(self):
        for day in (f"2026-01-{n:02d}" for n in range(1, 10)):
            make_archive(self.dir, retention.DEFAULT_PREFIX + stamp(day))
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            self.assertEqual(retention.main([self.dir, "--dry-run"]), 0)
        self.assertIn("would delete:", output.getvalue())
        self.assertEqual(len(os.listdir(self.dir)), 9 * 2 + 1)
        first = retention.plan_deletions(self.dir)
        with contextlib.redirect_stdout(io.StringIO()):
            self.assertEqual(retention.main([self.dir]), 0)
        self.assertEqual(retention.plan_deletions(self.dir), [])
        for name in first:
            self.assertFalse(os.path.exists(os.path.join(self.dir, name)))

    def test_unsafe_archive_child_aborts_deletion(self):
        name = retention.DEFAULT_PREFIX + stamp("2020-01-01")
        path = make_archive(self.dir, name)
        os.symlink("/etc/passwd", os.path.join(path, "link"))
        with self.assertRaises(RuntimeError):
            retention.safe_remove_archive(self.dir, name)
        self.assertTrue(os.path.isdir(path))

    def test_filesystem_error_returns_nonzero(self):
        name = retention.DEFAULT_PREFIX + stamp("2020-01-01")
        make_archive(self.dir, name)
        make_archive(self.dir, retention.DEFAULT_PREFIX + stamp("2020-01-02"))
        with mock.patch.object(retention.os, "remove", side_effect=OSError("disk failure")):
            self.assertEqual(
                retention.main([self.dir, "--keep-daily", "0", "--keep-weekly", "0", "--keep-monthly", "0"]), 1
            )
        self.assertTrue(os.path.isdir(os.path.join(self.dir, name)))

    def test_concurrent_run_fails_fast(self):
        script = os.path.join(os.path.dirname(__file__), "backup_retention.py")
        lock = retention.acquire_lock(self.dir, 0.1)
        try:
            result = subprocess.run(
                [sys.executable, script, self.dir, "--lock-timeout", "0.1"],
                capture_output=True, text=True, check=False,
            )
        finally:
            import fcntl
            fcntl.flock(lock.fileno(), fcntl.LOCK_UN)
            lock.close()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("lock is busy", result.stderr)


if __name__ == "__main__":
    unittest.main()
