"""Tests of tools/gocache_prune.py: python3 -m unittest tools/test_gocache_prune.py"""
import os
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))
import gocache_prune

NOW = 1_000_000_000.0


def entry(cache: Path, name: str, size: int, age_s: float) -> Path:
    d = cache / name[:2]
    d.mkdir(exist_ok=True)
    p = d / name
    p.write_bytes(b"x" * size)
    os.utime(p, (NOW - age_s, NOW - age_s))
    return p


class Prune(unittest.TestCase):
    def test_deletes_the_longest_unused_first_until_under_the_cap(self):
        with tempfile.TemporaryDirectory() as t:
            c = Path(t)
            old, mid, new = entry(c, "aa-old", 100, 9000), entry(c, "bb-mid", 100, 7000), entry(c, "cc-new", 100, 5000)
            self.assertEqual(gocache_prune.prune(c, 150, 3600, NOW), 200)
            self.assertEqual([p.exists() for p in (old, mid, new)], [False, False, True])

    def test_under_the_cap_deletes_nothing(self):
        with tempfile.TemporaryDirectory() as t:
            p = entry(Path(t), "aa-x", 100, 9000)
            self.assertEqual(gocache_prune.prune(Path(t), 100, 3600, NOW), 0)
            self.assertTrue(p.exists())

    def test_a_file_used_within_the_minimum_age_stays_even_over_the_cap(self):
        with tempfile.TemporaryDirectory() as t:
            fresh = entry(Path(t), "aa-fresh", 100, 60)
            self.assertEqual(gocache_prune.prune(Path(t), 0, 3600, NOW), 0)
            self.assertTrue(fresh.exists())

    def test_top_level_files_are_not_entries(self):
        with tempfile.TemporaryDirectory() as t:
            (Path(t) / "README").write_text("x" * 100)
            self.assertEqual(gocache_prune.prune(Path(t), 0, 0, NOW), 0)
            self.assertTrue((Path(t) / "README").exists())


if __name__ == "__main__":
    unittest.main()
