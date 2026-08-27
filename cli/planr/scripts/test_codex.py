from __future__ import annotations

import pathlib
import shutil
import subprocess
import tempfile
import unittest
from copy import deepcopy
from unittest import mock

from codex import (
    DEFAULT_FIXTURE,
    DEFAULT_REASONING,
    DEFAULT_LANGUAGE,
    INSTALLED_AGENTS_FILE,
    SUPPORTED_LANGUAGES,
    agents_file_for,
    agents_path,
    agents_variants,
    prompt_variants,
    resolve_variant,
    fixture_language,
    resolve_language,
    set_workspace_language,
    PLANR_CONFIG_FILE,
    FIXTURE_LABELS,
    prompt_file_for,
    FIXTURE_TEST_FILE,
    copy_plan_artifacts,
    describe_item,
    elapsed,
    final_state,
    format_usage,
    final_response_from_items,
    install_fixture,
    load_initial_prompt,
    token_usage_summary,
)
from common import PLANS_DIR, HarnessError, fixture_dir, load_harness_config


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


# Every fixture has to satisfy the same contract, so each of these runs over
# all of them rather than only the default one.
ALL_FIXTURES = sorted(FIXTURE_LABELS)


class InstallFixtureTest(unittest.TestCase):
    """The agent must see repository content, never the evaluation's own files."""

    def workspace_for(
        self, fixture: str, language: str = DEFAULT_LANGUAGE, agents_variant: str | None = None
    ) -> pathlib.Path:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        workspace = pathlib.Path(directory.name) / "repo"
        install_fixture(fixture, workspace, language, agents_variant)
        return workspace

    def test_no_fixture_prefixed_file_reaches_the_workspace(self) -> None:
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                workspace = self.workspace_for(fixture)
                self.assertEqual([p.name for p in workspace.rglob("FIXTURE.*")], [])
                for language in SUPPORTED_LANGUAGES:
                    self.assertFalse((workspace / prompt_file_for(language)).exists())
                # The acceptance script grades the result; an agent that could
                # read it could satisfy the checks instead of the request.
                self.assertFalse((workspace / FIXTURE_TEST_FILE).exists())

    def test_instructions_are_installed_under_the_expected_name(self) -> None:
        # The agent must find the instructions for the run's language, not for
        # whichever language happens to be the fixture's default.
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    workspace = self.workspace_for(fixture, language)
                    target = workspace / INSTALLED_AGENTS_FILE
                    self.assertTrue(target.is_file(), f"{INSTALLED_AGENTS_FILE} missing")
                    self.assertEqual(
                        target.read_text(encoding="utf-8"),
                        agents_path(fixture, language).read_text(encoding="utf-8"),
                    )

    def test_every_non_fixture_file_is_copied(self) -> None:
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                source_dir = fixture_dir(fixture)
                workspace = self.workspace_for(fixture)
                expected = {
                    path.relative_to(source_dir)
                    for path in source_dir.rglob("*")
                    if path.is_file() and not path.name.startswith("FIXTURE.")
                }
                self.assertTrue(expected, f"{fixture} has no repository content")
                for relative in expected:
                    self.assertTrue((workspace / relative).is_file(), f"{relative} missing")

    def test_instructions_variant_is_independent_of_the_language(self) -> None:
        # An A/B run pins the documents to one language while swapping only the
        # instructions, so the two choices must not be wired together.
        workspace = self.workspace_for(DEFAULT_FIXTURE, "ko", agents_variant="en")
        self.assertEqual(
            (workspace / INSTALLED_AGENTS_FILE).read_text(encoding="utf-8"),
            agents_path(DEFAULT_FIXTURE, "en").read_text(encoding="utf-8"),
        )
        self.assertEqual(fixture_language(workspace), "ko")

    def test_greenfield_fixture_ships_no_go_scaffolding(self) -> None:
        # Its whole point is that the agent sets the project up itself.
        workspace = self.workspace_for("codex-greenfield")
        self.assertEqual([p.name for p in workspace.rglob("*.go")], [])
        self.assertFalse((workspace / "go.mod").exists())


class CopyPlanArtifactsTest(unittest.TestCase):
    """The workspace is temporary, so the plans must be copied out of it."""

    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        root = pathlib.Path(directory.name)
        self.workspace = root / "workspace"
        self.run_dir = root / "run"
        self.run_dir.mkdir(parents=True)
        plan = self.workspace / "plans-active" / "00-demo"
        (plan / "phases").mkdir(parents=True)
        (plan / "PLAN.md").write_text("---\nplan_status: done\n---\n", encoding="utf-8")
        (plan / "phases" / "00-initial.md").write_text("---\nstatus: done\n---\n", encoding="utf-8")
        (self.workspace / "demo.md").write_text(
            '---\nplan_name: demo\ndescription: "x"\n---\n# GOALS\n', encoding="utf-8"
        )
        (self.workspace / "README.md").write_text("# not a plan\n", encoding="utf-8")
        (self.workspace / "main.go").write_text("package main\n", encoding="utf-8")
        for name in (".git", ".harness", "bin"):
            (self.workspace / name).mkdir()

    def test_plan_directories_and_drafts_are_copied(self) -> None:
        copied = copy_plan_artifacts(self.workspace, self.run_dir)
        self.assertIn("demo.md", copied)
        self.assertIn(str(pathlib.Path("plans-active/00-demo/PLAN.md")), copied)
        self.assertIn(str(pathlib.Path("plans-active/00-demo/phases/00-initial.md")), copied)
        self.assertTrue((self.run_dir / PLANS_DIR / "plans-active" / "00-demo" / "PLAN.md").is_file())

    def test_source_files_and_plain_markdown_are_left_out(self) -> None:
        copied = copy_plan_artifacts(self.workspace, self.run_dir)
        self.assertNotIn("README.md", copied)
        self.assertNotIn("main.go", copied)
        self.assertFalse((self.run_dir / PLANS_DIR / "README.md").exists())

    def test_plan_directories_are_found_by_shape_not_by_name(self) -> None:
        # A fixture may name its plans_dirs anything; the on-disk signature is
        # a child directory holding PLAN.md.
        renamed = self.workspace / "custom-plan-home" / "00-demo"
        renamed.mkdir(parents=True)
        (renamed / "PLAN.md").write_text("---\n---\n", encoding="utf-8")
        copied = copy_plan_artifacts(self.workspace, self.run_dir)
        self.assertIn(str(pathlib.Path("custom-plan-home/00-demo/PLAN.md")), copied)

    def test_workspace_without_plans_copies_nothing(self) -> None:
        (self.workspace / "demo.md").unlink()
        shutil.rmtree(self.workspace / "plans-active")
        self.assertEqual(copy_plan_artifacts(self.workspace, self.run_dir), [])


class FinalStateConfigTest(unittest.TestCase):
    def test_completion_commands_and_paths_come_from_config(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            workspace = root / "workspace"
            run_dir = root / "run"
            workspace.mkdir()
            (run_dir / "state").mkdir(parents=True)
            config = deepcopy(load_harness_config())
            config["tool"]["name"] = "widget"
            config["tool"]["binary"] = "bin/widget"
            config["completion"]["command"] = ["inspect", "--json"]
            config["completion"]["state_file"] = "state/end.json"
            config["completion"]["status_command"] = ["summary"]

            def completed(args: list[str], **_: object) -> subprocess.CompletedProcess[str]:
                return subprocess.CompletedProcess(args, 0, "{}\n")

            with mock.patch("codex.run_command", side_effect=completed) as run, mock.patch(
                "codex.copy_plan_artifacts", return_value=[]
            ), mock.patch("codex.run_fixture_test", return_value=None):
                self.assertEqual(final_state(workspace, run_dir, "fixture", config), (0, None))

            commands = [call.args[0] for call in run.call_args_list]
            self.assertEqual(commands[1], [str(workspace / "bin" / "widget"), "inspect", "--json"])
            self.assertEqual(commands[2], [str(workspace / "bin" / "widget"), "summary"])
            self.assertTrue((run_dir / "state" / "end.json").is_file())


class FixtureTestScriptTest(unittest.TestCase):
    def test_greenfield_ships_an_acceptance_script(self) -> None:
        self.assertTrue((fixture_dir("codex-greenfield") / FIXTURE_TEST_FILE).is_file())

    def test_every_acceptance_script_is_valid_bash(self) -> None:
        for fixture in ALL_FIXTURES:
            script = fixture_dir(fixture) / FIXTURE_TEST_FILE
            if not script.is_file():
                continue
            with self.subTest(fixture=fixture):
                result = subprocess.run(
                    ["bash", "-n", str(script)], capture_output=True, text=True
                )
                self.assertEqual(result.returncode, 0, result.stderr)

    def test_acceptance_script_reads_the_workspace_from_the_environment(self) -> None:
        # The harness passes the workspace this way; a script that hardcoded a
        # path would silently grade the wrong directory.
        script = (fixture_dir("codex-greenfield") / FIXTURE_TEST_FILE).read_text(encoding="utf-8")
        self.assertIn("PLANR_EVAL_WORKSPACE", script)


# Each language states "keep going until it is done" in its own words; the
# request is worthless to the harness without it, so each phrasing is pinned.
FINISH_ON_OWN = {"en": "without stopping to ask me", "ko": "끝까지"}


class InitialPromptTest(unittest.TestCase):
    def test_loads_from_every_fixture_and_language(self) -> None:
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    self.assertTrue(load_initial_prompt(fixture, language).strip())

    def test_reads_as_a_user_request_not_harness_policy(self) -> None:
        # The planr workflow belongs in AGENTS.md; if it leaks back into the
        # prompt the run stops measuring whether the agent finds it there.
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    prompt = load_initial_prompt(fixture, language)
                    for policy in ("planr", "phase", "--force"):
                        self.assertNotIn(policy, prompt, f"{policy!r} belongs in AGENTS.md")

    def test_asks_the_agent_to_finish_on_its_own(self) -> None:
        # Nothing nudges the agent after this message, so the request itself
        # has to say "keep going until it is done".
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    prompt = load_initial_prompt(fixture, language)
                    self.assertIn(FINISH_ON_OWN[language], prompt)

    def test_default_fixture_is_a_known_one(self) -> None:
        self.assertIn(DEFAULT_FIXTURE, FIXTURE_LABELS)


class InstalledInstructionsTest(unittest.TestCase):
    def test_every_language_has_instructions(self) -> None:
        # A missing translation would only surface as a failed run, so it is
        # checked here instead.
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    path = agents_path(fixture, language)
                    self.assertTrue(path.is_file(), f"{path} missing")

    def test_carries_the_planr_workflow(self) -> None:
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    agents = agents_path(fixture, language).read_text(encoding="utf-8")
                    for policy in ("planr new", "planr apply", "planr edit", "planr overview", "phase done", "--force"):
                        self.assertIn(policy, agents)

    def test_is_independent_of_this_repository(self) -> None:
        # These instructions must drop into any repository unchanged, so they
        # may not name anything that only exists in this sample project.
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                source_dir = fixture_dir(fixture)
                names = {
                    path.name
                    for path in source_dir.rglob("*")
                    if path.is_file() and not path.name.startswith("FIXTURE.")
                }
                for language in SUPPORTED_LANGUAGES:
                    agents = agents_path(fixture, language).read_text(encoding="utf-8")
                    for name in names - {".planr.yaml", "README.md", ".gitignore"}:
                        self.assertNotIn(name, agents, f"{name!r} only exists in this fixture")
                self.assertTrue(names, "fixture has no repository content to check against")


if __name__ == "__main__":
    unittest.main()


class VariantSelectionTest(unittest.TestCase):
    """Prompt and instructions are chosen independently, for A/B comparison."""

    def test_every_fixture_offers_a_prompt_variant_per_language(self) -> None:
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                self.assertLessEqual(set(SUPPORTED_LANGUAGES), set(prompt_variants(fixture)))

    def test_instructions_come_from_the_shared_library(self) -> None:
        # The point of moving them above the fixtures: no fixture carries its
        # own copy, so every fixture sees the same set.
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                self.assertEqual(agents_variants(fixture), agents_variants(DEFAULT_FIXTURE))
                self.assertEqual([p.name for p in fixture_dir(fixture).glob("FIXTURE.AGENTS.*")], [])
                for language in SUPPORTED_LANGUAGES:
                    self.assertEqual(agents_path(fixture, language).parent.name, "agents")

    def test_a_fixture_local_file_overrides_a_shared_variant(self) -> None:
        # A fixture may specialize one variant without forking the document.
        fixture = fixture_dir(DEFAULT_FIXTURE)
        override = fixture / agents_file_for("ab-test")
        override.write_text("local\n", encoding="utf-8")
        self.addCleanup(override.unlink)
        self.assertEqual(agents_path(DEFAULT_FIXTURE, "ab-test"), override)
        self.assertIn("ab-test", agents_variants(DEFAULT_FIXTURE))

    def test_variants_default_to_the_run_language(self) -> None:
        for language in SUPPORTED_LANGUAGES:
            with self.subTest(language=language):
                self.assertEqual(resolve_variant("prompt", ["en", "ko"], None, language), language)
                self.assertEqual(resolve_variant("prompt", ["en", "ko", "terse"], "terse", language), "terse")

    def test_unknown_variant_is_rejected_with_the_available_ones(self) -> None:
        with self.assertRaises(HarnessError) as caught:
            resolve_variant("agents", ["en", "ko"], "strict", "en")
        self.assertIn("en, ko", str(caught.exception))

    def test_the_shared_library_is_not_a_fixture(self) -> None:
        self.assertNotIn("agents", FIXTURE_LABELS)


class ResolveLanguageTest(unittest.TestCase):
    """`--language` overrides; otherwise the fixture's planr setting decides."""

    def test_override_wins_over_the_fixture_setting(self) -> None:
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    self.assertEqual(resolve_language(fixture, language), language)

    def test_falls_back_to_the_fixture_setting(self) -> None:
        for fixture in ALL_FIXTURES:
            with self.subTest(fixture=fixture):
                configured = fixture_language(fixture_dir(fixture))
                self.assertIsNotNone(configured, "fixture no longer pins a language")
                self.assertEqual(resolve_language(fixture, None), configured)

    def test_rejects_an_unsupported_language(self) -> None:
        with self.assertRaises(HarnessError):
            resolve_language(DEFAULT_FIXTURE, "fr")


class FixtureLanguageTest(unittest.TestCase):
    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.directory = pathlib.Path(directory.name)

    def write_config(self, contents: str) -> pathlib.Path:
        (self.directory / PLANR_CONFIG_FILE).write_text(contents, encoding="utf-8")
        return self.directory

    def test_reads_the_top_level_setting(self) -> None:
        self.assertEqual(fixture_language(self.write_config("language: ko\nplans_dirs:\n  - p\n")), "ko")

    def test_ignores_a_nested_key_of_the_same_name(self) -> None:
        # Only planr's own top-level setting counts; an indented `language:`
        # belongs to some other block.
        self.assertIsNone(fixture_language(self.write_config("hooks:\n  language: ko\n")))

    def test_absent_config_and_absent_key_are_unset(self) -> None:
        self.assertIsNone(fixture_language(self.directory))
        self.assertIsNone(fixture_language(self.write_config("plans_dirs:\n  - p\n")))


class SetWorkspaceLanguageTest(unittest.TestCase):
    """An override has to reach planr too, not only the instructions."""

    def setUp(self) -> None:
        directory = tempfile.TemporaryDirectory()
        self.addCleanup(directory.cleanup)
        self.workspace = pathlib.Path(directory.name)

    def config(self) -> str:
        return (self.workspace / PLANR_CONFIG_FILE).read_text(encoding="utf-8")

    def test_replaces_an_existing_setting_and_keeps_the_rest(self) -> None:
        (self.workspace / PLANR_CONFIG_FILE).write_text(
            "language: ko\nplans_dirs:\n  - plans-active\n", encoding="utf-8"
        )
        set_workspace_language(self.workspace, "en")
        self.assertIn("language: en", self.config())
        self.assertNotIn("language: ko", self.config())
        self.assertIn("plans-active", self.config())

    def test_adds_the_setting_when_absent(self) -> None:
        (self.workspace / PLANR_CONFIG_FILE).write_text("plans_dirs:\n  - p\n", encoding="utf-8")
        set_workspace_language(self.workspace, "ko")
        self.assertIn("language: ko", self.config())
        self.assertIn("plans_dirs", self.config())

    def test_creates_the_config_when_missing(self) -> None:
        set_workspace_language(self.workspace, "ko")
        self.assertEqual(self.config(), "language: ko\n")

    def test_install_pins_planr_to_the_run_language(self) -> None:
        # The whole point of the override: instructions and generated documents
        # must not end up in different languages.
        for fixture in ALL_FIXTURES:
            for language in SUPPORTED_LANGUAGES:
                with self.subTest(fixture=fixture, language=language):
                    directory = tempfile.TemporaryDirectory()
                    self.addCleanup(directory.cleanup)
                    workspace = pathlib.Path(directory.name) / "repo"
                    install_fixture(fixture, workspace, language)
                    self.assertEqual(fixture_language(workspace), language)
