# Working instructions

Plan the work with `planr` and follow that plan. `planr` is already installed
on `PATH`.

`planr` is a CLI that splits one piece of work into phases and keeps them as
Markdown documents. You write down what to do and in what order first, then
update each phase as you start and finish it, so the progress stays recorded in
the documents.

## Commands

```sh
planr new <kebab-name> --description "short description, 200 characters or fewer" # create a plan draft
planr add <draft-file> # validate the draft and register it as a plan
planr overview # progress summary for every plan
planr status # remaining phases and pending dependencies in detail
planr phase start <plan-name> <number> # begin a phase
planr phase done <plan-name> <number> # complete a phase
planr phase add <plan-name> <title> --work "..." --done-when "..." # add a phase to an open plan
```

The draft `planr new` creates has the sections `GOALS`, `SCOPE`, `CONTEXT`,
`PHASES`, `VERIFICATION`, `ORDERING` and `NEXT`, in that order. Each phase puts
`phase`, `slug`, `status` and `depends_on` in the YAML fence after its title,
and fills in the two subsections below it (planned work and completion
conditions) under exactly the headings the draft uses. Follow the structure the
draft already has; if the format is wrong, `planr add` says what is wrong and
refuses to register it.

The draft contains `TODO(planr)` markers. Every one of them must be replaced
with real content before it registers, and `planr add` reports all remaining
markers at once with their line numbers. A phase's `depends_on` lists other
phases in the same plan, by number or by slug (`[0]`, `[initial-work]`, or a
mix). The comment at the top of the draft summarizes the rules for each field.

## Workflow

1. Read the request and the existing code and tests first, and work out what is
   needed.
2. Before changing any code, create a draft with `planr new`, fill in the
   goals, verification method and phases to match the actual work, and register
   it with `planr add`. Split phases into units that can each be verified
   independently.
3. Check the result with `planr overview`.
4. For each phase, repeat:
   `planr phase start` → implement → verify (tests) → **commit the changes** →
   `planr phase done`
5. If the plan diverges from reality as you go, add a phase with
   `planr phase add` or correct the plan documents so they match the real work.
6. When every phase is finished, confirm with `planr overview` that they are
   all `done`.

## Rules

- `planr phase done` fails when there are uncommitted source changes. Commit
  first. Do not bypass this check with `--force`. Drafts and plan directories
  created by `planr` do not count as source changes, so they need not be
  committed.
- `planr phase start` and `planr phase done` fail when a prerequisite phase is
  not `done` yet. Work in the order you planned, and do not bypass it with
  `--force`.
- Keep the plan documents and the code and tests up to date together. Updating
  only the plan with no implementation, or implementing without moving the
  phase status, is not acceptable.
- Update plan documents and `.planr.yaml` through `planr` commands. Do not
  hand-edit them to fix up state.
