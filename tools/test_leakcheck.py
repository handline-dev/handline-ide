"""Tests of tools/leakcheck.py: python3 -m unittest tools/test_leakcheck.py"""
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).parent))
import leakcheck


class EndRemovesItsDirectory(unittest.TestCase):
    def test_a_leaking_run_with_nothing_running_is_removed(self):
        with tempfile.TemporaryDirectory() as base:
            run = leakcheck.new_run(Path(base) / "hlc")
            (run / "tmp" / "leftover").write_text("x")
            self.assertEqual(leakcheck.end(run), 1)
            self.assertFalse(run.exists())

    def test_a_read_only_tree_is_removed(self):
        with tempfile.TemporaryDirectory() as base:
            run = leakcheck.new_run(Path(base) / "hlc")
            mod = run / "tmp" / "mod" / "pkg"
            mod.mkdir(parents=True)
            (mod / "f.go").write_text("x")
            (mod / "f.go").chmod(0o444)
            mod.chmod(0o555)
            leakcheck.end(run)
            self.assertFalse(run.exists())

    def test_a_run_with_a_live_process_is_kept(self):
        with tempfile.TemporaryDirectory() as base:
            run = leakcheck.new_run(Path(base) / "hlc")
            p = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(30)", str(run)])
            try:
                self.assertEqual(leakcheck.end(run), 1)
                self.assertTrue(run.exists())
            finally:
                p.kill()
                p.wait()


class SweepKept(unittest.TestCase):
    def setUp(self):
        self.root = Path(tempfile.mkdtemp()).resolve()
        self.addCleanup(subprocess.run, ["rm", "-rf", str(self.root)])

    def run_dir(self, name: str, old: bool = True, owner: int | None = None) -> Path:
        p = self.root / f"hlc.{name}"
        (p / "tmp").mkdir(parents=True)
        if owner is not None:
            (p / "owner").write_text(str(owner))
        if old:
            t = time.time() - leakcheck.KEEP_S - 60
            os.utime(p, (t, t))
        return p

    def holding(self, run: Path) -> None:
        """A process that names run on its command line, alive until the test ends."""
        proc = subprocess.Popen(["sh", "-c", "sleep 30; :", "sh", str(run / "tmp" / "home")])
        self.addCleanup(proc.wait)
        self.addCleanup(proc.kill)
        for _ in range(100):  # until ps shows the child as it runs, not as the fork
            if str(run) in subprocess.run(["ps", "-ww", "-o", "command=", "-p", str(proc.pid)], capture_output=True, text=True).stdout:
                return
            time.sleep(0.02)
        self.fail("the holding process never showed")

    def test_an_old_run_no_process_names_goes(self):
        p = self.run_dir("old")
        leakcheck.sweep_kept(self.root)
        self.assertFalse(p.exists())

    def test_a_run_under_a_day_old_stays(self):
        p = self.run_dir("fresh", old=False)
        leakcheck.sweep_kept(self.root)
        self.assertTrue(p.exists())

    def test_a_run_a_process_names_on_its_command_line_stays(self):
        p = self.run_dir("argv")
        self.holding(p)
        leakcheck.sweep_kept(self.root)
        self.assertTrue(p.exists())

    def test_a_run_whose_owner_still_runs_stays(self):
        p = self.run_dir("owned", owner=os.getpid())
        leakcheck.sweep_kept(self.root)
        self.assertTrue(p.exists())

    def test_a_run_whose_owner_has_exited_goes(self):
        gone = subprocess.Popen(["true"])
        gone.wait()
        p = self.run_dir("orphaned", owner=gone.pid)
        leakcheck.sweep_kept(self.root)
        self.assertFalse(p.exists())

    def test_nothing_goes_when_the_processes_cannot_be_listed(self):
        p = self.run_dir("blind")
        for failed in (subprocess.CompletedProcess([], 1, "", "ps: error"), OSError("no ps")):
            with mock.patch.object(leakcheck.subprocess, "run", side_effect=[failed]):
                leakcheck.sweep_kept(self.root)
            self.assertTrue(p.exists())


if __name__ == "__main__":
    unittest.main()
