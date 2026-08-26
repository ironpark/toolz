from __future__ import annotations

import pathlib
import tempfile
import unittest

from common import HarnessError
from scenario import substitute


class SubstituteTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = pathlib.Path(directory.name)

    def write(self, contents: str) -> pathlib.Path:
        path = self.directory / "PLAN.md"
        path.write_text(contents, encoding="utf-8")
        return path

    def test_replaces_every_occurrence(self) -> None:
        path = self.write("status: planned\nstatus: conditional\n")
        substitute(path, r"status: (?:planned|conditional)", "status: done")
        self.assertEqual(path.read_text(encoding="utf-8"), "status: done\nstatus: done\n")

    def test_missing_match_is_an_error(self) -> None:
        # The scenario fabricates plan states by rewriting frontmatter, so a
        # silent no-op would produce a misleading report instead of failing.
        path = self.write("plan_status: done\n")
        with self.assertRaises(HarnessError):
            substitute(path, r"plan_status: in-progress", "plan_status: done")


if __name__ == "__main__":
    unittest.main()
