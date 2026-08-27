#!/usr/bin/env python3
"""Reproduce the checkout release scenario and print status/overview output.

The scenario needs states a single `planr apply` cannot produce on its own: a
finished plan, a plan waiting on another plan's phase, a partially finished
plan, an unrelated finished plan that `status` hides.  Every one of them is
produced by shaping planr's *input* -- one draft per plan, with the plan
dependency written into the draft's frontmatter -- and then driving the real
`planr phase` commands.  Nothing here edits a document planr generated.

That distinction is what keeps the scenario honest.  Rewriting generated
frontmatter would couple the runner to an internal file format and would let
the scenario display arrangements the CLI itself would refuse -- a phase marked
done ahead of its dependencies, for instance.  The draft format, by contrast,
is a documented interface, so shaping the input breaks loudly and for a real
reason when it changes.
"""

from __future__ import annotations

import pathlib
import shutil

from common import (
    HarnessError,
    build_tool,
    fixture_dir,
    init_git_repository,
    load_harness_config,
    make_run_dir,
    remove_runs,
    run_command,
    write_run_harness_config,
)


FIXTURE_NAME = "plan-scenario"
RUN_LABEL = "scenario"
DRAFT = "checkout-v2.md"
# The fixture draft defines these three phases; the scenario completes them by
# number, so a fixture with fewer phases must fail rather than silently produce
# a thinner scenario.
PHASE_COUNT = 3

# Registration order fixes the numeric prefix of each plan directory, so the
# list order below is what makes AUTH `00-auth-foundation` and so on.
PLANS = ["auth-foundation", "checkout-v2", "payment-adapter", "legacy-report", "partial-rollout"]
AUTH, CHECKOUT, PAYMENT, LEGACY, ROLLOUT = PLANS

# Plan-level dependencies, written into each draft before registration.
# checkout waits on a phase that ends up done, so it never appears in `wait`
# but does keep the finished auth plan visible in `status`; payment waits on a
# phase that stays open, so it is the one blocking entry.
DEPENDENCIES = {
    CHECKOUT: [f"{AUTH}#2"],
    PAYMENT: [f"{CHECKOUT}#1"],
}


def planr(workspace: pathlib.Path, *args: str) -> str:
    result = run_command([str(workspace / "bin" / "planr"), *args], cwd=workspace)
    if result.returncode != 0:
        raise HarnessError(f"planr {' '.join(args)} failed (exit {result.returncode}):\n{result.stdout}")
    return result.stdout


def draft_body(workspace: pathlib.Path) -> str:
    """Return the fixture draft with its frontmatter removed.

    Each plan gets the same body under its own name and dependencies, so the
    scenario reads as five plans rather than five copies of one file.
    """

    path = workspace / DRAFT
    contents = path.read_text(encoding="utf-8")
    if not contents.startswith("---\n"):
        raise HarnessError(f"{path}: draft has no frontmatter; the fixture may have changed")
    end = contents.find("\n---\n", 3)
    if end < 0:
        raise HarnessError(f"{path}: draft frontmatter is unterminated")
    return contents[end + len("\n---\n") :]


def write_draft(workspace: pathlib.Path, plan: str, body: str) -> pathlib.Path:
    """Write a draft for one plan, carrying its plan dependencies."""

    front = [f"plan_name: {plan}"]
    dependencies = DEPENDENCIES.get(plan, [])
    if dependencies:
        front.append("depends_on: [" + ", ".join(dependencies) + "]")
    path = workspace / f"{plan}.md"
    path.write_text("---\n" + "\n".join(front) + "\n---\n" + body, encoding="utf-8")
    return path


def complete_phases(workspace: pathlib.Path, plan: str, count: int) -> None:
    """Complete the first `count` phases of a plan in order.

    Order matters: planr refuses to complete a phase whose dependencies are
    still open, which is exactly the guarantee the scenario should respect.
    """

    for phase in range(count):
        planr(workspace, "phase", "done", plan, str(phase))


def prepare_workspace() -> pathlib.Path:
    config = load_harness_config()
    workspace = make_run_dir(RUN_LABEL)
    shutil.copytree(fixture_dir(FIXTURE_NAME), workspace, dirs_exist_ok=True)
    write_run_harness_config(workspace, config)
    # bin/ is listed in the fixture's `ignore`, so the binary under test never
    # counts as an uncommitted source change during `phase done`.
    build_tool(config, workspace / "bin" / "planr")
    init_git_repository(workspace, message="scenario baseline")
    return workspace


def build_scenario(workspace: pathlib.Path) -> None:
    body = draft_body(workspace)
    for plan in PLANS:
        planr(workspace, "apply", str(write_draft(workspace, plan, body).name))

    # A finished dependency, and an unrelated finished plan that `status` hides.
    complete_phases(workspace, AUTH, PHASE_COUNT)
    complete_phases(workspace, LEGACY, PHASE_COUNT)
    # Some phases done, some outstanding.
    complete_phases(workspace, ROLLOUT, 1)
    # Work under way on the plan whose dependency is already satisfied.
    planr(workspace, "phase", "start", CHECKOUT, "0")


def run_scenario() -> int:
    workspace = prepare_workspace()
    build_scenario(workspace)
    print(f"Run directory: {workspace}\n")
    print("Detailed status:")
    print(planr(workspace, "status"), end="")
    print("\nOverview:")
    print(planr(workspace, "overview"), end="")
    print("\nCompletion records:")
    print(planr(workspace, "notes"), end="")
    return 0


def clean() -> int:
    print(f"Removed {remove_runs(RUN_LABEL)} scenario run(s)")
    return 0


def main(argv: list[str]) -> int:
    """Run the scenario. Errors are reported by the caller in main.py."""

    if argv == ["clean"]:
        return clean()
    if argv:
        raise HarnessError("scenario accepts no options other than `clean`")
    return run_scenario()
