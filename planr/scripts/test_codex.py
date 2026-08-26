from __future__ import annotations

import pathlib
import tempfile
import unittest

from codex import (
    DEFAULT_REASONING,
    FIXTURE_INSTALLED_FILES,
    FIXTURE_NAME,
    FIXTURE_PROMPT_FILE,
    describe_item,
    elapsed,
    format_usage,
    final_response_from_items,
    install_fixture,
    load_initial_prompt,
    token_usage_summary,
)
from common import fixture_dir


class HarnessHelpersTest(unittest.TestCase):
    def test_token_usage_prefers_cumulative_thread_breakdown(self) -> None:
        # `last` covers only the most recent model request; an agentic session
        # issues one per tool call, so only `total` describes the whole run.
        usage = token_usage_summary(
            {
                "last": {
                    "inputTokens": 11,
                    "cachedInputTokens": 3,
                    "outputTokens": 7,
                    "reasoningOutputTokens": 2,
                    "totalTokens": 18,
                },
                "total": {
                    "inputTokens": 99,
                    "cachedInputTokens": 50,
                    "outputTokens": 40,
                    "reasoningOutputTokens": 10,
                    "totalTokens": 139,
                },
            }
        )
        self.assertEqual(
            usage,
            {
                "input_tokens": 99,
                "cached_input_tokens": 50,
                "output_tokens": 40,
                "reasoning_output_tokens": 10,
                "total_tokens": 139,
            },
        )

    def test_token_usage_falls_back_to_last_when_total_absent(self) -> None:
        usage = token_usage_summary({"last": {"outputTokens": 7, "totalTokens": 18}})
        self.assertEqual(usage, {"output_tokens": 7, "total_tokens": 18})

    def test_final_response_uses_last_agent_message(self) -> None:
        items = [
            {"type": "agentMessage", "text": "first"},
            {"type": "commandExecution", "command": "go test ./..."},
            {"type": "agentMessage", "text": "last"},
        ]
        self.assertEqual(final_response_from_items(items), "last")

    def test_default_reasoning(self) -> None:
        self.assertEqual(DEFAULT_REASONING, "medium")


class DescribeItemTest(unittest.TestCase):
    def test_command_with_argv_list(self) -> None:
        line = describe_item({"type": "commandExecution", "command": ["planr", "overview"]})
        self.assertEqual(line, "cmd  planr overview")

    def test_failed_command_shows_exit_code(self) -> None:
        line = describe_item({"type": "commandExecution", "command": "go test ./...", "exitCode": 1})
        self.assertEqual(line, "cmd  go test ./... (exit 1)")

    def test_agent_message_is_collapsed_and_truncated(self) -> None:
        line = describe_item({"type": "agentMessage", "text": "a\n\n" + "b" * 200})
        self.assertTrue(line.startswith("say  a b"))
        self.assertTrue(line.endswith("…"))
        self.assertLessEqual(len(line), 120)

    def test_file_change_lists_paths(self) -> None:
        line = describe_item({"type": "fileChange", "changes": [{"path": "main.go"}]})
        self.assertEqual(line, "edit  main.go")

    def test_unknown_item_type_falls_back_to_its_raw_name(self) -> None:
        # A label table must not double as a filter: an SDK item type nobody
        # has labelled yet should still show up in the log.
        self.assertEqual(describe_item({"type": "somethingNew"}), "somethingNew")

    def test_untyped_item_is_skipped(self) -> None:
        self.assertIsNone(describe_item({"command": "ls"}))

    def test_known_item_without_payload_still_labels(self) -> None:
        self.assertEqual(describe_item({"type": "reasoning"}), "think")


class FormatUsageTest(unittest.TestCase):
    def test_missing_usage(self) -> None:
        self.assertEqual(format_usage(None), "tokens n/a")

    def test_total_is_derived_when_absent(self) -> None:
        self.assertEqual(
            format_usage({"input_tokens": 40000, "output_tokens": 5000}),
            "45.0k tokens (in 40.0k / out 5.0k)",
        )


class ElapsedTest(unittest.TestCase):
    def test_formats(self) -> None:
        self.assertEqual(elapsed(9), "0:09")
        self.assertEqual(elapsed(192), "3:12")
        self.assertEqual(elapsed(3725), "1:02:05")


class InstallFixtureTest(unittest.TestCase):
    """The agent must see repository content, never the evaluation's own files."""

    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.workspace = pathlib.Path(directory.name) / "repo"
        install_fixture(fixture_dir(FIXTURE_NAME), self.workspace)

    def test_no_fixture_prefixed_file_reaches_the_workspace(self) -> None:
        leaked = [p.name for p in self.workspace.rglob("FIXTURE.*")]
        self.assertEqual(leaked, [])

    def test_prompt_is_not_copied(self) -> None:
        self.assertFalse((self.workspace / FIXTURE_PROMPT_FILE).exists())

    def test_instructions_are_installed_under_the_expected_name(self) -> None:
        for source, installed in FIXTURE_INSTALLED_FILES.items():
            target = self.workspace / installed
            self.assertTrue(target.is_file(), f"{installed} missing")
            self.assertEqual(
                target.read_text(encoding="utf-8"),
                (fixture_dir(FIXTURE_NAME) / source).read_text(encoding="utf-8"),
            )

    def test_repository_content_is_copied(self) -> None:
        for name in ("main.go", "main_test.go", "go.mod", "README.md", ".planr.yaml"):
            self.assertTrue((self.workspace / name).is_file(), f"{name} missing")


class InitialPromptTest(unittest.TestCase):
    def test_loads_from_the_fixture(self) -> None:
        self.assertTrue(load_initial_prompt().strip())

    def test_reads_as_a_user_request_not_harness_policy(self) -> None:
        # The planr workflow belongs in AGENTS.md; if it leaks back into the
        # prompt the run stops measuring whether the agent finds it there.
        prompt = load_initial_prompt()
        for policy in ("planr", "phase", "--force"):
            self.assertNotIn(policy, prompt, f"{policy!r} belongs in AGENTS.md")

    def test_asks_the_agent_to_finish_on_its_own(self) -> None:
        # Nothing nudges the agent after this message, so the request itself
        # has to say "keep going until it is done".
        self.assertIn("끝까지", load_initial_prompt())


class InstalledInstructionsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.agents = (fixture_dir(FIXTURE_NAME) / "FIXTURE.AGENTS.md").read_text(encoding="utf-8")

    def test_carries_the_planr_workflow(self) -> None:
        for policy in ("planr new", "planr add", "planr overview", "phase done", "--force"):
            self.assertIn(policy, self.agents)

    def test_is_independent_of_this_repository(self) -> None:
        # These instructions must drop into any repository unchanged, so they
        # may not name anything that only exists in this sample project.
        fixture = fixture_dir(FIXTURE_NAME)
        names = {
            path.name
            for path in fixture.rglob("*")
            if path.is_file() and not path.name.startswith("FIXTURE.")
        }
        for name in names - {".planr.yaml", "README.md", ".gitignore"}:
            self.assertNotIn(name, self.agents, f"{name!r} only exists in this fixture")
        self.assertTrue(names, "fixture has no repository content to check against")


if __name__ == "__main__":
    unittest.main()
