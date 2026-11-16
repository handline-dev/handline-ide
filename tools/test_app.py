"""tools/app.mjs refuses `stop --all` without an agent name, which would match every instance that has no owner."""
import json
import os
import pathlib
import platform
import subprocess
import tempfile
import time
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


@unittest.skipUnless(platform.system() == "Darwin", "app-instance.mjs runs on a Mac")
class StoppedBySignal(unittest.TestCase):
    """A signal skips every finally, and the detached app and daemon would outlive the run."""

    def test_a_daemon_of_the_scratch_root_is_ended_when_the_run_is_terminated(self):
        script = """
import { spawn } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { disposeOnSignal, scratch } from %r;
const s = await scratch('hl-smoke-test-');
fs.copyFileSync('/bin/sleep', path.join(s.root, 'handlined'));
const d = spawn(path.join(s.root, 'handlined'), ['300'], { detached: true, stdio: 'ignore' });
d.unref();
disposeOnSignal(() => ({ ...s, pids: [] }));
console.log(s.root, d.pid);
setInterval(() => {}, 1000);
""" % str(APP.with_name("app-instance.mjs"))
        run = subprocess.Popen(["node", "--input-type=module", "-e", script], stdout=subprocess.PIPE, text=True)
        started = run.stdout.readline().split()
        self.assertEqual(len(started), 2, "the script did not start")
        root, pid = started
        try:
            run.terminate()
            self.assertEqual(run.wait(timeout=20), 143)
            for _ in range(50):
                if subprocess.run(["ps", "-p", pid], capture_output=True).returncode:
                    break
                time.sleep(0.1)
            else:
                self.fail("the daemon outlived the terminated run")
        finally:
            subprocess.run(["kill", "-9", pid], capture_output=True)
            subprocess.run(["rm", "-rf", root])


if __name__ == "__main__":
    unittest.main()
