#!/usr/bin/env python3
"""Reproduce the checkout release scenario and print status/overview output.

Registering the same draft several times only ever produces identical
in-progress plans, so the interesting `status` and `overview` states — a
completed plan, a plan waiting on another plan's phase, a partially finished
plan, a hidden unrelated plan — are fabricated by rewriting frontmatter after
registration.  That is the whole point of the scenario: it is a display
fixture, not a test of the state transitions themselves.
"""

from __future__ import annotations

import pathlib
import re
import shutil

from common import HarnessError, build_planr, fixture_dir, make_run_dir, remove_runs, run_command


FIXTURE_NAME = "plan-scenario"
RUN_LABEL = "scenario"
DRAFT = "checkout-v2.md"

# Every plan is registered from the same draft; the name decides its role below,
# and registration order fixes the numeric prefix of each plan directory.
PLANS = ["auth-foundation", "checkout-v2", "payment-adapter", "legacy-report", "partial-rollout"]
AUTH, CHECKOUT, PAYMENT, LEGACY, ROLLOUT = (
    f"{index:02d}-{plan}" for index, plan in enumerate(PLANS)
)


def substitute(path: pathlib.Path, pattern: str, replacement: str) -> None:
    contents = path.read_text(encoding="utf-8")
    updated, count = re.subn(pattern, replacement, contents)
    if count == 0:
        raise HarnessError(f"{path}: no match for {pattern!r}; the plan format may have changed")
    path.write_text(updated, encoding="utf-8")


def plan_dir(workspace: pathlib.Path, plan: str) -> pathlib.Path:
    return workspace / "plans-active" / plan


def complete_plan(workspace: pathlib.Path, plan: str) -> None:
    """Mark a plan and all of its phases done."""

    root = plan_dir(workspace, plan)
    substitute(root / "PLAN.md", r"plan_status: in-progress", "plan_status: done")
    for phase in sorted(root.glob("phases/*.md")):
        substitute(phase, r"status: (?:planned|conditional)", "status: done")


def complete_phase(workspace: pathlib.Path, plan: str, phase: str) -> None:
    substitute(plan_dir(workspace, plan) / "phases" / phase, r"status: planned", "status: done")


def add_dependency(workspace: pathlib.Path, plan: str, phase: str, dependency: str) -> None:
    substitute(
        plan_dir(workspace, plan) / "phases" / phase,
        r"depends_on: \[\]",
        f'depends_on:\n- "{dependency}"',
    )


def planr(workspace: pathlib.Path, *args: str) -> str:
    result = run_command([str(workspace / "planr"), *args], cwd=workspace)
    if result.returncode != 0:
        raise HarnessError(f"planr {' '.join(args)} failed (exit {result.returncode}):\n{result.stdout}")
    return result.stdout


def prepare_workspace() -> pathlib.Path:
    workspace = make_run_dir(RUN_LABEL)
    shutil.copytree(fixture_dir(FIXTURE_NAME), workspace, dirs_exist_ok=True)
    build_planr(workspace / "planr")
    return workspace


def build_scenario(workspace: pathlib.Path) -> None:
    for plan in PLANS:
        planr(workspace, "add", "--name", plan, DRAFT)

    # A finished dependency, and an unrelated finished plan that `status` hides.
    complete_plan(workspace, AUTH)
    complete_plan(workspace, LEGACY)
    # Some phases done, some outstanding.
    complete_phase(workspace, ROLLOUT, "00-api-contract.md")
    # A satisfied cross-plan wait, and one that is still blocking.
    add_dependency(workspace, CHECKOUT, "00-api-contract.md", f"{AUTH}#2")
    add_dependency(workspace, PAYMENT, "00-api-contract.md", f"{CHECKOUT}#1")


def run_scenario() -> int:
    workspace = prepare_workspace()
    build_scenario(workspace)
    print(f"Run directory: {workspace}\n")
    print("Detailed status:")
    print(planr(workspace, "status"), end="")
    print("\nOverview:")
    print(planr(workspace, "overview"), end="")
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
