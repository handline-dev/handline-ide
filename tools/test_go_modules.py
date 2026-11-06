"""tools/go-modules.py: which Go modules a change selects, on this repo's module graph and on a small one of its own."""
import importlib.util
import os
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("go_modules", os.path.join(os.path.dirname(__file__), "go-modules.py"))
go_modules = importlib.util.module_from_spec(spec)
spec.loader.exec_module(go_modules)


class Select(unittest.TestCase):
    def setUp(self):
        self.graph = go_modules.modules()

    def picked(self, *paths):
        return set(go_modules.select(list(paths), self.graph))

    def test_the_graph_is_read_from_the_go_mod_files(self):
        self.assertIn(".", self.graph)
        self.assertGreaterEqual(len(self.graph), 2)
        self.assertEqual(self.graph["."], set(self.graph) - {"."}, "the daemon requires every other module")

    def test_a_daemon_only_change_tests_the_daemon_alone(self):
        self.assertEqual(self.picked("go/internal/reads/reads.go", "go/cmd/hl/main.go"), {"."})

    def test_a_kit_change_tests_kit_and_every_module_that_requires_it(self):
        users = {d for d, needs in self.graph.items() if "kit" in needs}
        self.assertEqual(self.picked("go/kit/paths/paths.go"), users | {"kit"})
        self.assertNotIn("agentctl", self.picked("go/kit/paths/paths.go"), "agentctl does not require kit")

    def test_a_go_mod_change_tests_every_module(self):
        for p in ("go/kit/go.mod", "go/go.sum", "go/go.work", ".github/workflows/check.yml", "tools/test-go.sh"):
            self.assertEqual(self.picked(p), set(self.graph), p)

    def test_no_go_change_tests_no_module(self):
        self.assertEqual(self.picked("docs/a.md", "apps/web/src/App.tsx", "tools/app-smoke.mjs"), set())
        self.assertEqual(self.picked(), set())

    def test_what_the_go_tests_read_tests_every_module(self):
        self.assertEqual(self.picked("contract/spec/tasks.json"), set(self.graph))

    def test_what_one_module_s_tests_read_selects_that_module(self):
        self.assertIn("data", self.picked("skill/SKILL.md"))
        self.assertIn("data", self.picked("AGENTS.md"))
        self.assertNotIn("kit", self.picked("AGENTS.md"))

    def test_go_work_is_read_in_either_form_with_comments(self):
        with tempfile.TemporaryDirectory() as root:
            for d in ("a", "b", "c"):
                os.makedirs(os.path.join(root, "go", d))
                with open(os.path.join(root, "go", d, "go.mod"), "w") as f:
                    f.write(f"module {go_modules.PREFIX}/{d}\n")
            with open(os.path.join(root, "go", "go.mod"), "w") as f:
                f.write(f"module {go_modules.PREFIX}\n\nrequire {go_modules.PREFIX}/a v0.0.0\n\nrequire (\n\t{go_modules.PREFIX}/b v0.0.0 // b\n)\n\n"
                        f"replace {go_modules.PREFIX}/c => ./c\n")
            with open(os.path.join(root, "go", "go.work"), "w") as f:
                f.write('go 1.26.0\n\nuse ./c // the third\n\nuse (\n\t.\n\t"./a" // quoted\n\t./b\n)\n')
            self.assertEqual(go_modules.modules(root), {".": {"a", "b"}, "a": set(), "b": set(), "c": set()})

    def test_a_dependent_of_a_dependent_is_selected_and_each_says_why(self):
        graph = {".": {"a", "b", "c"}, "a": set(), "b": {"a"}, "c": {"b"}, "d": set()}
        why = go_modules.select(["go/a/x.go"], graph)
        self.assertEqual(set(why), {".", "a", "b", "c"})
        self.assertEqual(why["a"], "go/a/x.go changed")
        self.assertIn("requires", why["c"])

    def test_a_module_whose_name_starts_another_s_is_not_taken_for_it(self):
        graph = {".": {"kit", "kitchen"}, "kit": set(), "kitchen": set()}
        self.assertEqual(set(go_modules.select(["go/kitchen/x.go"], graph)), {".", "kitchen"})


if __name__ == "__main__":
    unittest.main()
