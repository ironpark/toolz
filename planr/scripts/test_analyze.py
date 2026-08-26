from __future__ import annotations

import json
import pathlib
import tempfile
import unittest

from analyze import (
    find_paths_outside,
    build_transcript,
    error_excerpt,
    failed_executions,
    fixture_check_rows,
    markdown_report,
    parse_fixture_checks,
    planr_commands,
    read_session,
    token_breakdown_rows,
    token_shares,
    worktree_state,
)

from common import SESSION_LOG


def command_event(item_id: str, command: str, output: str, exit_code: int, status: str) -> dict:
    return {
        "type": f"item.{status}",
        "notification": {
            "payload": {
                "item": {
                    "type": "commandExecution",
                    "id": item_id,
                    "command": command,
                    "aggregatedOutput": output,
                    "exitCode": exit_code,
                    "status": status,
                }
            }
        },
    }


class SessionReadingTest(unittest.TestCase):
    def setUp(self) -> None:
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.run_dir = pathlib.Path(self.directory.name)

    def write_events(self, events: list[dict]) -> None:
        (self.run_dir / SESSION_LOG).write_text(
            "".join(json.dumps(event) + "\n" for event in events), encoding="utf-8"
        )

    def test_executions_keep_each_command_with_its_own_output(self) -> None:
        self.write_events(
            [
                command_event("a", "echo first", "FIRST", 0, "started"),
                command_event("b", "echo second", "SECOND", 1, "started"),
                command_event("a", "echo first", "FIRST", 0, "completed"),
            ]
        )
        executions = list(read_session(self.run_dir).executions.values())
        self.assertEqual(
            [(item.command, item.output, item.exit_code) for item in executions],
            [("echo first", "FIRST", 0), ("echo second", "SECOND", 1)],
        )

    def test_transcript_renders_output_next_to_its_command(self) -> None:
        self.write_events(
            [
                command_event("a", "echo first", "FIRST", 0, "completed"),
                command_event("b", "echo second", "SECOND", 1, "completed"),
            ]
        )
        transcript = build_transcript(self.run_dir, read_session(self.run_dir))
        self.assertLess(transcript.index("echo first"), transcript.index("FIRST"))
        self.assertLess(transcript.index("FIRST"), transcript.index("echo second"))
        self.assertLess(transcript.index("echo second"), transcript.index("SECOND"))

    def test_usage_snapshots_are_cumulative_not_additive(self) -> None:
        # Each record is a running thread total, so repeating one must not
        # inflate the run's token count.
        snapshot = {"input_tokens": 100, "output_tokens": 40, "total_tokens": 140}
        self.write_events(
            [
                {"type": "session.progress", "usage": dict(snapshot)},
                {"type": "session.completed", "usage": dict(snapshot)},
            ]
        )
        usage = read_session(self.run_dir).usage
        self.assertEqual(usage["output_tokens"], 40)
        self.assertEqual(usage["total_tokens"], 140)


class PlanrCommandCountTest(SessionReadingTest):
    def test_wrapper_and_inner_command_count_once(self) -> None:
        self.write_events(
            [command_event("a", "/bin/zsh -lc 'planr overview'", "plans-active/", 0, "completed")]
        )
        self.assertEqual(len(planr_commands(read_session(self.run_dir))), 1)


class FailedCommandTest(SessionReadingTest):
    def test_only_non_zero_commands_are_listed(self) -> None:
        self.write_events(
            [
                command_event("a", "echo ok", "fine", 0, "completed"),
                command_event("b", "planr add draft.md", "2026/08/27 01:17:19 boom", 1, "completed"),
            ]
        )
        failures = failed_executions(read_session(self.run_dir))
        self.assertEqual(len(failures), 1)
        self.assertEqual(failures[0]["exit_code"], 1)
        # Numbered as in the transcript, so the two can be cross-referenced.
        self.assertEqual(failures[0]["index"], 2)
        self.assertEqual(failures[0]["planr_actions"], ["add"])

    def test_shell_failure_is_not_attributed_to_planr(self) -> None:
        self.write_events([command_event("a", "gofmt -w main.go", "boom", 2, "completed")])
        self.assertEqual(failed_executions(read_session(self.run_dir))[0]["planr_actions"], [])

    def test_running_command_without_exit_code_is_not_a_failure(self) -> None:
        self.write_events(
            [
                {
                    "type": "item.started",
                    "notification": {
                        "payload": {
                            "item": {"type": "commandExecution", "id": "a", "command": "sleep 1"}
                        }
                    },
                }
            ]
        )
        self.assertEqual(failed_executions(read_session(self.run_dir)), [])


class FailureReportSectionTest(unittest.TestCase):
    def render(self, failures: list[dict]) -> str:
        return markdown_report(
            {"failed_commands": failures, "session": {"commands_executed": 5}},
            pathlib.Path("/run"),
        )

    def test_planr_and_other_failures_get_separate_headings(self) -> None:
        report = self.render(
            [
                {"index": 1, "command": "planr add d.md", "exit_code": 1, "planr_actions": ["add"], "error": "boom"},
                {"index": 2, "command": "gofmt -w x.go", "exit_code": 2, "planr_actions": [], "error": "bang"},
            ]
        )
        self.assertIn("(`planr` 1건, 기타 1건)", report)
        planr_heading = report.index("### `planr` 명령 실패")
        other_heading = report.index("### 기타 명령 실패")
        self.assertLess(planr_heading, report.index("planr add d.md"))
        self.assertLess(report.index("planr add d.md"), other_heading)
        self.assertLess(other_heading, report.index("gofmt -w x.go"))

    def test_empty_group_is_marked_rather_than_dropped(self) -> None:
        report = self.render(
            [{"index": 1, "command": "gofmt -w x.go", "exit_code": 2, "planr_actions": [], "error": "bang"}]
        )
        self.assertIn("(`planr` 0건, 기타 1건)", report)
        section = report[report.index("### `planr` 명령 실패") : report.index("### 기타 명령 실패")]
        self.assertIn("없음.", section)

    def test_run_without_failures_says_so(self) -> None:
        self.assertIn("0건. 실행된 모든 명령이 exit 0으로 끝났습니다.", self.render([]))


class TokenBreakdownTest(unittest.TestCase):
    TOKENS = {
        "input_tokens": 800,
        "cached_input_tokens": 600,
        "output_tokens": 200,
        "reasoning_output_tokens": 50,
        "total_tokens": 1000,
    }

    def test_subsets_are_shares_of_their_parent_not_of_the_total(self) -> None:
        rows = "\n".join(token_breakdown_rows(self.TOKENS))
        self.assertIn("| input | 800 | 80.0% (전체 대비) |", rows)
        self.assertIn("| └ cached input | 600 | 75.0% (input 대비) |", rows)
        self.assertIn("| output | 200 | 20.0% (전체 대비) |", rows)
        self.assertIn("| └ reasoning output | 50 | 25.0% (output 대비) |", rows)

    def test_parent_shares_sum_to_the_whole(self) -> None:
        shares = token_shares(self.TOKENS)
        self.assertAlmostEqual(shares["input_of_total"] + shares["output_of_total"], 1.0)
        self.assertEqual(shares["cached_of_input"], 0.75)
        self.assertEqual(shares["reasoning_of_output"], 0.25)

    def test_mismatch_between_total_and_its_parts_is_flagged(self) -> None:
        rows = "\n".join(
            token_breakdown_rows({"input_tokens": 10, "output_tokens": 10, "total_tokens": 40})
        )
        self.assertIn("일치하지 않습니다", rows)

    def test_missing_usage_does_not_divide_by_zero(self) -> None:
        self.assertEqual(token_breakdown_rows({}), ["토큰 사용량을 읽지 못했습니다."])
        self.assertIsNone(token_shares({})["input_of_total"])


class ErrorExcerptTest(unittest.TestCase):
    def test_root_cause_wins_over_downstream_fallout(self) -> None:
        # Steps chained with `;` keep running after the first failure, so the
        # last line is the fallout and the first error line is the cause.
        output = "\n".join(
            [
                "some ordinary output",
                "2026/08/27 01:17:19 NEXT description must not be empty",
                "2026/08/27 01:17:19 no plans directories found: /tmp/a, /tmp/b",
                '2026/08/27 01:17:19 plan "demo" not found',
            ]
        )
        self.assertTrue(
            error_excerpt(output).startswith("2026/08/27 01:17:19 NEXT description must not be empty")
        )

    def test_go_test_failure_names_the_test_not_the_bare_summary(self) -> None:
        # `go test` ends with three uninformative `FAIL` lines; the useful part
        # is the named failure above them.
        output = "\n".join(
            [
                "--- FAIL: TestAddListDoneRemoveAndIDs (0.00s)",
                "    main_test.go:31: add requires exactly one non-empty title",
                "FAIL",
                "FAIL\texample.com/tasks\t0.364s",
                "FAIL",
            ]
        )
        excerpt = error_excerpt(output)
        self.assertTrue(excerpt.startswith("--- FAIL: TestAddListDoneRemoveAndIDs"))
        self.assertIn("main_test.go:31", excerpt)

    def test_falls_back_to_the_tail_when_nothing_looks_like_an_error(self) -> None:
        self.assertEqual(error_excerpt("a\nb\nc\nd", limit=2), "c\nd")


class FixtureCheckTest(unittest.TestCase):
    OUTPUT = "\n".join(
        [
            "some noise the script printed",
            "CHECK\tbuild\tPASS\t",
            "CHECK\ttag-filter\tFAIL\t--tag home 결과가 비어 있음",
            "CHECK\tsummary\tFAIL\t1/2 실패",
        ]
    )

    def test_only_well_formed_check_lines_are_parsed(self) -> None:
        checks = parse_fixture_checks(self.OUTPUT + "\nCHECK\tbroken\nCHECK\tx\tMAYBE\t")
        self.assertEqual([check["name"] for check in checks], ["build", "tag-filter", "summary"])
        self.assertEqual(checks[1]["detail"], "--tag home 결과가 비어 있음")

    def test_summary_is_a_headline_not_a_table_row(self) -> None:
        rows = "\n".join(fixture_check_rows(parse_fixture_checks(self.OUTPUT), "1"))
        self.assertIn("1/2 통과", rows)
        self.assertIn("스크립트 보고: 1/2 실패", rows)
        self.assertIn("| `tag-filter` | **FAIL** |", rows)
        self.assertNotIn("| `summary` |", rows)

    def test_fixture_without_a_script_says_so(self) -> None:
        self.assertEqual(
            fixture_check_rows([], ""), ["이 픽스처에는 인수 검사 스크립트가 없습니다."]
        )

    def test_script_that_produced_no_checks_is_flagged(self) -> None:
        rows = "\n".join(fixture_check_rows([], "2"))
        self.assertIn("CHECK 줄을 찾지 못했습니다", rows)


class WorktreeStateTest(unittest.TestCase):
    def test_untracked_planr_output_does_not_make_a_run_dirty(self) -> None:
        state = worktree_state(
            {"final_git_status": "?? draft.md\n?? plans-active/", "final_git_status_tracked": ""}
        )
        self.assertEqual(state, "clean (untracked만 2건)")

    def test_uncommitted_tracked_change_is_dirty(self) -> None:
        state = worktree_state(
            {"final_git_status": " M main.go", "final_git_status_tracked": " M main.go"}
        )
        self.assertEqual(state, "dirty")

    def test_run_without_tracked_capture_falls_back_to_full_status(self) -> None:
        state = worktree_state({"final_git_status": " M main.go", "final_git_status_tracked": None})
        self.assertEqual(state, "dirty")


if __name__ == "__main__":
    unittest.main()


class FindPathsOutsideTest(unittest.TestCase):
    WORKSPACE = "/var/folders/ab/planr-codex-xyz"

    def test_relative_paths_are_not_absolute(self) -> None:
        # `./...`, `./tasks` and the `**/.git/**` glob appear in nearly every
        # Go or ripgrep command; reading them as absolute made this signal fire
        # on every run and say nothing.
        commands = [
            "go test ./... && go vet ./...",
            "go build -o tasks . && ./tasks add 'buy milk'",
            "rg --files -g '!**/.git/**'",
            "cd ../sibling && ls",
        ]
        self.assertEqual(find_paths_outside(commands, self.WORKSPACE), [])

    def test_paths_inside_the_workspace_are_allowed(self) -> None:
        commands = [f"cat {self.WORKSPACE}/main.go", "ls /usr/bin", "mktemp -d /tmp/x"]
        self.assertEqual(find_paths_outside(commands, self.WORKSPACE), [])

    def test_reports_a_real_escape(self) -> None:
        commands = [f"cat {self.WORKSPACE}/main.go", "cat /Users/someone/secrets.txt"]
        self.assertEqual(find_paths_outside(commands, self.WORKSPACE), ["/Users/someone/secrets.txt"])
