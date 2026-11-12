import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

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


if __name__ == "__main__":
    unittest.main()
