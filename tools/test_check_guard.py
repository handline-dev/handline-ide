"""Tests of what tools/check.sh does before it starts: python3 -m unittest tools/test_check_guard.py"""
import os
import subprocess
import unittest
from pathlib import Path

ROOT = Path(__file__).parent.parent


class DiskFloor(unittest.TestCase):
    def run_check(self, floor: str):
        env = {**os.environ, "CHECK_MIN_FREE_GB": floor, "CHECK_FULL": "1"}
        env.pop("GITHUB_ACTIONS", None)
        return subprocess.run([str(ROOT / "tools/check.sh"), "--group", "go"], cwd=ROOT, env=env, capture_output=True, text=True, timeout=60)

    def test_below_the_floor_it_does_not_start_and_says_why(self):
        r = self.run_check("100000000")
        self.assertEqual(r.returncode, 1)
        self.assertIn("free (floor 100000000 GB): not starting", r.stderr)


if __name__ == "__main__":
    unittest.main()
