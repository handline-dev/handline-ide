#!/usr/bin/env python3
"""A read added to the spec gets its cache entry; one removed loses it (generate.py gen_app_data)."""
import copy
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import generate as g


def spec():
    return g.join(g.parts())


def reads(s):
    return next(t for p, t in g.gen_app_data(s).items() if p.name == "reads.ts")


class AppData(unittest.TestCase):
    def test_added_read_gets_its_entry(self):
        s = spec()
        read = copy.deepcopy(next(m for m in s["methods"] if m["name"] == "agent.get"))
        read["name"] = "fixture.thing"
        s["methods"].append(read)
        self.assertIn('"fixture.thing": { resource: ', reads(s))

    def test_removed_read_loses_its_entry(self):
        s = spec()
        self.assertIn('"agent.get": {', reads(s))
        s["methods"] = [m for m in s["methods"] if m["name"] != "agent.get"]
        self.assertNotIn('"agent.get": {', reads(s))

    def test_uncached_read_is_listed_as_not_cached(self):
        s = spec()
        read = copy.deepcopy(next(m for m in s["methods"] if m["name"] == "agent.get"))
        read["name"], read["x-app-cache"] = "fixture.raw", False
        s["methods"].append(read)
        text = reads(s)
        self.assertNotIn('"fixture.raw": {', text)
        self.assertIn('"fixture.raw"', text.split("NOT_CACHED")[1].split(";")[0])


if __name__ == "__main__":
    unittest.main()
