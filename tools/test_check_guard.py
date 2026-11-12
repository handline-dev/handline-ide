"""Tests of what tools/check.sh does before it starts: python3 -m unittest tools/test_check_guard.py"""
import os
import tempfile
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


class HeavySlot(unittest.TestCase):
    def fake_hl(self, d: Path, usage: str) -> dict:
        hl = d / "hl"
        hl.write_text(f'#!/bin/sh\n[ $# -eq 1 ] && {{ echo "{usage}" >&2; exit 2; }}\necho "$@" > {d}/asked\n')
        hl.chmod(0o755)
        env = {**os.environ, "PATH": f"{d}:{os.environ['PATH']}", "CHECK_MIN_FREE_GB": "0", "CHECK_FULL": "1"}
        env.pop("GITHUB_ACTIONS", None)
        env.pop("CHECK_HEAVY_HELD", None)
        return env

    def run_check(self, env):
        return subprocess.run([str(ROOT / "tools/check.sh"), "--group", "go"], cwd=ROOT, env=env, capture_output=True, text=True, timeout=60)

    def test_it_runs_itself_inside_a_heavy_slot(self):
        with tempfile.TemporaryDirectory() as t:
            self.run_check(self.fake_hl(Path(t), "usage: hl use <heavy|simulator|browser> -- cmd"))
            self.assertTrue((Path(t) / "asked").read_text().startswith("use heavy -- "))


if __name__ == "__main__":
    unittest.main()
