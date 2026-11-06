#!/usr/bin/env python3
"""A method added to the spec yields its hook; one removed loses it (generate.py gen_app_data)."""
import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import generate as g


def spec():
    return g.join(g.parts())


def hooks(s):
    return next(t for p, t in g.gen_app_data(s).items() if p.name == "hooks.ts")


class AppData(unittest.TestCase):
    def test_added_read_and_write_get_hooks(self):
        s = spec()
        read = copy.deepcopy(next(m for m in s["methods"] if m["name"] == "agent.get"))
        read["name"] = "fixture.thing"
        write = copy.deepcopy(next(m for m in s["methods"] if m["name"] == "task.new"))
        write["name"] = "fixture.make"
        s["methods"] += [read, write]
        text = hooks(s)
        self.assertIn("export const useFixtureThing = (params: CallParams<\"fixture.thing\">", text)
        self.assertIn("export const useFixtureMake = ", text)

    def test_removed_method_loses_its_hook(self):
        s = spec()
        self.assertIn("useAgentGet", hooks(s))
        s["methods"] = [m for m in s["methods"] if m["name"] != "agent.get"]
        self.assertNotIn("useAgentGet", hooks(s))

    def test_uncached_write_has_no_hook(self):
        s = spec()
        self.assertNotIn("useSessionInput", hooks(s))


if __name__ == "__main__":
    unittest.main()
