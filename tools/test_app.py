"""tools/app.mjs refuses `stop --all` without an agent name, which would match every instance that has no owner."""
import json
import os
import pathlib
import platform
import subprocess
import tempfile
import unittest

APP = pathlib.Path(__file__).with_name("app.mjs")


@unittest.skipUnless(platform.system() == "Darwin", "app.mjs runs on a Mac")
class StopAll(unittest.TestCase):
    def run_stop(self, home, agent):
        env = {k: v for k, v in os.environ.items() if k != "HANDLINE_AGENT"}
        env["HANDLINE_HOME"] = home
        if agent:
            env["HANDLINE_AGENT"] = agent
        return subprocess.run(["node", str(APP), "stop", "--all"], env=env, capture_output=True, text=True)

    def test_without_an_agent_nothing_is_stopped(self):
        with tempfile.TemporaryDirectory() as home:
            d = pathlib.Path(home, "app-instances")
            d.mkdir()
            f = d / "other.json"
            f.write_text(json.dumps({"owner": "", "pids": []}))
            r = self.run_stop(home, "")
            self.assertEqual(r.returncode, 1)
            self.assertIn("HANDLINE_AGENT", r.stderr)
            self.assertTrue(f.exists())

    def test_with_an_agent_only_its_own_are_stopped(self):
        with tempfile.TemporaryDirectory() as home:
            d = pathlib.Path(home, "app-instances")
            d.mkdir()
            (d / "mine.json").write_text(json.dumps({"owner": "ann", "pids": [], "root": home + "/gone", "home": home + "/gone/home", "debugPort": 1}))
            (d / "other.json").write_text(json.dumps({"owner": "", "pids": []}))
            r = self.run_stop(home, "ann")
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertFalse((d / "mine.json").exists())
            self.assertTrue((d / "other.json").exists())


if __name__ == "__main__":
    unittest.main()
