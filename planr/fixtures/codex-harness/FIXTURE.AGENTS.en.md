# Working instructions

Plan the work with `planr` and follow that plan. `planr` is already installed
on `PATH`.

`planr` splits one piece of work into phases and keeps the plan as Markdown
documents. Use its JSON and stdin/stdout interfaces when an operation can stay
in memory; do not open plan documents or `.planr.yaml` directly.

## Commands

```sh
planr schema --json # document contract and valid status/dependency values
planr new <kebab-name> --description "short description, 200 characters or fewer" --json
planr apply --stdin # apply the completed Markdown sent on stdin
planr overview --json # progress summary for every plan
planr status --json # remaining phases and pending dependencies
planr show <plan-name> --all --json # all plan documents and phase data
planr edit <plan-name>#<number> --json # check out one phase into memory
planr edit <plan-name> --section plan --json # check out an editable plan section
planr new <plan-name>#<title> --json # produce a new phase draft
planr phase start <plan-name> <number> # begin a phase
planr phase done <plan-name> <number> # complete a phase
```

`new --json` returns a selector and a `template` string. Fill every
`TODO(planr)` marker, then send that string to `planr apply --stdin`. A phase
draft contains its work, done-when, `depends_on`, `status`,
`entry_condition`, `perf_phase`, and editable slug; `apply` assigns the phase
number as the next number after the largest existing one.

Send a document by writing it to a scratch file and redirecting it, not by
embedding it in a shell quote: plan text contains backticks, quotes, and blank
lines that a quoted `-c` string mangles, and shell expansion can execute the
command examples inside your own document.

```sh
cat > .planr-draft <<'PLANR'
<the full document>
PLANR
planr apply --stdin < .planr-draft && rm .planr-draft
```

Remove the scratch file once it is applied; an untracked file left in the
repository blocks `planr phase done`.

Add `--json` to `apply` when it fails. The failure comes back as structured
errors naming the rule and the section or phase to fix, which is faster than
re-reading the document.

`edit --json` returns the checkout document and its `planr_base` hash. Preserve
the frontmatter identity fields and send the edited `document` to
`planr apply --stdin`. If another command changed the target meanwhile,
application is refused; check it out again. Phase status changes belong only
to `phase start`, `phase done`, `phase reset`, or `phase set`.

The plan draft has `GOALS`, `SCOPE`, `CONTEXT`, `PHASES`, `VERIFICATION`,
`ORDERING` and `NEXT`, in that order. Each phase puts `phase`, `slug`, `status`
and `depends_on` in the YAML fence after its title, and fills in planned work
and completion conditions under the headings already present. Follow the
structure returned by `new`; `apply --json` reports validation failures with
rules, sections, phases, and line numbers.

## Workflow

1. Read the request and existing code and tests first, and work out what is
   needed.
2. Create a plan with `new --json`, fill its template, and apply it through
   stdin. Split phases into units that can each be verified independently.
3. Check the result with `overview --json`, `status --json`, and
   `show --all --json`.
4. For each phase, repeat:
   `planr phase start` → implement → verify (tests) → **commit the changes** →
   `planr phase done`.
5. If the plan diverges, create a phase draft with `new plan#title`, or check
   out and apply the relevant section or phase with `edit`.
6. When every phase is finished, confirm with `overview --json` that they are
   all `done`.

## Rules

- `planr phase done` fails when there are uncommitted source changes. Commit
  first. Do not bypass this check with `--force`.
- `planr phase start` and `planr phase done` fail when a prerequisite phase is
  not `done` yet. Work in the order you planned, and do not bypass it with
  `--force`.
- Keep the plan documents and the code and tests up to date together. Updating
  only the plan with no implementation, or implementing without moving the
  phase status, is not acceptable.
- Treat the phase checklist and `plan_status` as derived. A plan section
  checkout exposes the checklist as a protected marker; leave that marker
  unchanged.
