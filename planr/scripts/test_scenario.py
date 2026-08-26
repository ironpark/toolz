from __future__ import annotations

import pathlib
import tempfile
import unittest

from common import HarnessError
from scenario import CHECKOUT, DEPENDENCIES, DRAFT, PLANS, draft_body, write_draft


class DraftBodyTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.workspace = pathlib.Path(directory.name)

    def write_fixture(self, contents: str) -> None:
        (self.workspace / DRAFT).write_text(contents, encoding="utf-8")

    def test_strips_frontmatter_and_keeps_the_body(self) -> None:
        self.write_fixture("---\nplan_name: checkout-v2\n---\n# GOALS\n\nShip it.\n")
        self.assertEqual(draft_body(self.workspace), "# GOALS\n\nShip it.\n")

    def test_body_containing_a_rule_is_not_truncated(self) -> None:
        # A horizontal rule in Markdown looks exactly like a frontmatter
        # terminator, so only the first one may end the frontmatter.
        self.write_fixture("---\nplan_name: x\n---\n# GOALS\n\n---\n\nmore\n")
        self.assertEqual(draft_body(self.workspace), "# GOALS\n\n---\n\nmore\n")

    def test_missing_frontmatter_is_an_error(self) -> None:
        # The scenario builds every plan from this body; a fixture that no
        # longer carries frontmatter must fail loudly rather than register five
        # plans named after the draft file.
        self.write_fixture("# GOALS\n")
        with self.assertRaises(HarnessError):
            draft_body(self.workspace)

    def test_unterminated_frontmatter_is_an_error(self) -> None:
        self.write_fixture("---\nplan_name: checkout-v2\n")
        with self.assertRaises(HarnessError):
            draft_body(self.workspace)


class WriteDraftTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.workspace = pathlib.Path(directory.name)

    def test_names_the_plan_and_carries_its_dependencies(self) -> None:
        path = write_draft(self.workspace, CHECKOUT, "# GOALS\n")
        contents = path.read_text(encoding="utf-8")
        self.assertEqual(path.name, f"{CHECKOUT}.md")
        self.assertIn(f"plan_name: {CHECKOUT}", contents)
        self.assertIn("depends_on: [" + ", ".join(DEPENDENCIES[CHECKOUT]) + "]", contents)
        self.assertTrue(contents.endswith("---\n# GOALS\n"))

    def test_plan_without_dependencies_omits_the_key(self) -> None:
        # An empty depends_on list would be written back as `depends_on: []`,
        # which planr prunes; leaving the key out keeps input and output alike.
        plan = next(name for name in PLANS if name not in DEPENDENCIES)
        contents = write_draft(self.workspace, plan, "# GOALS\n").read_text(encoding="utf-8")
        self.assertNotIn("depends_on", contents)

    def test_every_declared_dependency_names_a_scenario_plan(self) -> None:
        # A dependency on a plan the scenario never registers would show up as
        # `(not found)` in the output instead of the wait it is meant to show.
        for plan, dependencies in DEPENDENCIES.items():
            self.assertIn(plan, PLANS)
            for dependency in dependencies:
                self.assertIn(dependency.split("#", 1)[0], PLANS)


if __name__ == "__main__":
    unittest.main()
