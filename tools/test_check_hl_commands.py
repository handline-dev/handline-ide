import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import check_hl_commands as c  # noqa: E402


class CheckHlCommands(unittest.TestCase):
    def found(self, text):
        verbs, subs = c.tables()
        with tempfile.NamedTemporaryFile("w", suffix=".md", dir=c.ROOT, delete=False) as f:
            f.write(text)
        try:
            return c.problems(os.path.basename(f.name), verbs, subs)
        finally:
            os.unlink(f.name)

    def test_real_commands_pass(self):
        self.assertEqual(self.found("Run `hl task done T1 \"x\"`, `hl device list` and `hl say human \"hi\"`.\n"), [])

    def test_a_missing_verb_or_subcommand_is_named(self):
        self.assertEqual(len(self.found("`hl doctor` and `hl task frobnicate`\n")), 2)

    def test_prose_outside_backticks_is_not_read(self):
        self.assertEqual(self.found("hl doctor is gone, said in words\n"), [])

    def test_the_repository_names_only_real_commands(self):
        self.assertEqual(c.main(["check"]), 0)


if __name__ == "__main__":
    unittest.main()
