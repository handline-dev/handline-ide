"""Tests of tools/leakcheck.py: python3 -m unittest tools/test_leakcheck.py"""
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import leakcheck  # noqa: E402


class SweepKept(unittest.TestCase):
    def test_an_old_run_no_process_names_goes_and_the_rest_stay(self):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d).resolve()
            old, fresh, held = (root / f"hlc.{n}" for n in ("old", "fresh", "held"))
            for p in (old, fresh, held):
                (p / "tmp").mkdir(parents=True)
            day_ago = time.time() - leakcheck.KEEP_S - 60
            for p in (old, held):
                os.utime(p, (day_ago, day_ago))
            (root / "hlc.file").touch()  # not a run's directory
            os.utime(root / "hlc.file", (day_ago, day_ago))
            proc = subprocess.Popen(["sh", "-c", "sleep 30; :", "sh", str(held)])  # the path on its command line
            try:
                for _ in range(100):  # until ps shows the child's own command line
                    ps = subprocess.run(["ps", "-ww", "-o", "command=", "-p", str(proc.pid)], capture_output=True, text=True)
                    if str(held) in ps.stdout:
                        break
                    time.sleep(0.02)
                leakcheck.sweep_kept(root)
            finally:
                proc.kill()
                proc.wait()
            self.assertFalse(old.exists())
            self.assertTrue(fresh.exists())
            self.assertTrue(held.exists())
            self.assertTrue((root / "hlc.file").exists())


if __name__ == "__main__":
    unittest.main()
