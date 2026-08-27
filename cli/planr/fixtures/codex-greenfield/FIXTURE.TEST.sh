#!/usr/bin/env bash
# Acceptance checks for the codex-greenfield fixture.
#
# The harness runs this after the agent's session ends. It never enters the
# agent's workspace as a file, so the agent cannot read the checks and write
# code that satisfies them literally; it only sees the request in
# FIXTURE.PROMPT.md.
#
# Contract with the harness:
#   cwd                   a scratch directory, safe to write in
#   PLANR_EVAL_WORKSPACE  the agent's workspace (read-only by convention)
#   stdout                one `CHECK<TAB>name<TAB>PASS|FAIL<TAB>detail` line per
#                         check, which analyze.py turns into a report table
#   exit status           0 when every check passed, 1 otherwise
#
# Each check runs in its own directory and uses the default ./tasks.json, so a
# defensible difference in where `--file` is accepted cannot cascade into
# unrelated failures. One dedicated check covers `--file` itself and accepts
# either flag placement.

set -u

WORKSPACE=${PLANR_EVAL_WORKSPACE:?PLANR_EVAL_WORKSPACE must point at the agent workspace}
SCRATCH=$PWD
BIN=$SCRATCH/tasks-under-test
failures=0
checks=0

pass() {
	checks=$((checks + 1))
	printf 'CHECK\t%s\tPASS\t%s\n' "$1" "${2-}"
}

fail() {
	checks=$((checks + 1))
	failures=$((failures + 1))
	printf 'CHECK\t%s\tFAIL\t%s\n' "$1" "$(printf '%s' "${2-}" | tr '\n\t' '  ' | cut -c1-300)"
}

# A check that cannot run because an earlier step failed is a failure, not a
# silent skip: a report that hides them reads as better than the run was.
skip_rest() {
	local reason=$1
	shift
	for name in "$@"; do
		fail "$name" "실행 못 함: $reason"
	done
}

workdir() {
	local dir=$SCRATCH/case-$1
	rm -rf "$dir"
	mkdir -p "$dir"
	printf '%s' "$dir"
}

# Run the built CLI, capturing stdout and stderr separately so checks can tell
# "printed an error" from "printed a result".
run() {
	local dir=$1
	shift
	(cd "$dir" && "$BIN" "$@" >"$dir/out" 2>"$dir/err")
}

json_check() {
	python3 -c "$1" "$2" 2>&1
}

# ---------------------------------------------------------------- go module --

if [ -f "$WORKSPACE/go.mod" ]; then
	module=$(sed -n 's/^module[[:space:]][[:space:]]*//p' "$WORKSPACE/go.mod" | head -1)
	if [ "$module" = "example.com/tasks" ]; then
		pass go-module "module $module"
	else
		fail go-module "module 이름이 example.com/tasks가 아님: ${module:-없음}"
	fi
else
	fail go-module "go.mod 없음"
fi

# -------------------------------------------------------------------- build --

build_output=$(cd "$WORKSPACE" && go build -o "$BIN" . 2>&1)
if [ $? -eq 0 ] && [ -x "$BIN" ]; then
	pass build ""
else
	fail build "$build_output"
	skip_rest "빌드 실패" add-list json-shape id-sequence id-not-reused done-status \
		list-hides-done status-filter tag-filter empty-list unknown-id unknown-command \
		file-flag bad-due
	printf 'CHECK\t%s\t%s\t%s\n' summary FAIL "$failures/$checks 실패"
	exit 1
fi

# ------------------------------------------------------------------ go test --

test_output=$(cd "$WORKSPACE" && go test ./... 2>&1)
if [ $? -eq 0 ]; then
	pass go-test ""
else
	fail go-test "$test_output"
fi

# ------------------------------------------------------------------ add/list --

dir=$(workdir add-list)
run "$dir" add "우유 사기" --due 2026-09-01 --tag home
if [ $? -ne 0 ]; then
	fail add-list "add 실패: $(cat "$dir/err")"
elif run "$dir" list && grep -q "우유 사기" "$dir/out"; then
	pass add-list ""
else
	fail add-list "list 출력에 제목이 없음: $(cat "$dir/out" "$dir/err")"
fi

# ---------------------------------------------------------------- json shape --

dir=$(workdir json-shape)
run "$dir" add "첫 번째" --due 2026-09-01 --tag home
run "$dir" list --json
if [ $? -ne 0 ]; then
	fail json-shape "list --json 실패: $(cat "$dir/err")"
else
	detail=$(json_check '
import json, sys
raw = open(sys.argv[1], encoding="utf-8").read()
items = json.loads(raw)
if not isinstance(items, list):
    raise SystemExit("최상위가 배열이 아님")
if not items:
    raise SystemExit("배열이 비어 있음")
item = items[0]
for key, kind in (("id", int), ("title", str), ("status", str), ("tags", list), ("due", str)):
    if key not in item:
        raise SystemExit(f"{key} 필드 없음")
    if not isinstance(item[key], kind) or isinstance(item[key], bool):
        raise SystemExit(f"{key} 타입이 {kind.__name__}가 아님: {type(item[key]).__name__}")
if item["status"] not in ("open", "done"):
    raise SystemExit(f"status 값이 open/done이 아님: {item['status']}")
' "$dir/out")
	if [ -z "$detail" ]; then
		pass json-shape ""
	else
		fail json-shape "$detail"
	fi
fi

# -------------------------------------------------------------- id behaviour --

dir=$(workdir id-sequence)
run "$dir" add "첫 번째"
run "$dir" add "두 번째"
run "$dir" list --json
detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
ids = sorted(item["id"] for item in items)
if ids != [1, 2]:
    raise SystemExit(f"id가 1,2가 아님: {ids}")
' "$dir/out")
if [ -z "$detail" ]; then
	pass id-sequence ""
else
	fail id-sequence "$detail"
fi

dir=$(workdir id-not-reused)
run "$dir" add "첫 번째"
run "$dir" add "두 번째"
run "$dir" rm 2
run "$dir" add "세 번째"
run "$dir" list --json
detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
ids = sorted(item["id"] for item in items)
if 2 in ids:
    raise SystemExit(f"지운 id 2가 재사용됨: {ids}")
if ids != [1, 3]:
    raise SystemExit(f"id가 1,3이 아님: {ids}")
' "$dir/out")
if [ -z "$detail" ]; then
	pass id-not-reused ""
else
	fail id-not-reused "$detail"
fi

# ------------------------------------------------------------ done behaviour --

dir=$(workdir done-status)
run "$dir" add "끝낼 일"
run "$dir" done 1
if [ $? -ne 0 ]; then
	fail done-status "done 실패: $(cat "$dir/err")"
	fail list-hides-done "실행 못 함: done 실패"
	fail status-filter "실행 못 함: done 실패"
else
	run "$dir" list --status all --json
	detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
statuses = {item["id"]: item["status"] for item in items}
if statuses.get(1) != "done":
    raise SystemExit(f"id 1이 done이 아님: {statuses}")
' "$dir/out")
	if [ -z "$detail" ]; then
		pass done-status ""
	else
		fail done-status "$detail"
	fi

	run "$dir" add "남은 일"
	run "$dir" list --json
	detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
ids = sorted(item["id"] for item in items)
if 1 in ids:
    raise SystemExit(f"기본 list에 done이 포함됨: {ids}")
if ids != [2]:
    raise SystemExit(f"기본 list가 open만 보여주지 않음: {ids}")
' "$dir/out")
	if [ -z "$detail" ]; then
		pass list-hides-done ""
	else
		fail list-hides-done "$detail"
	fi

	run "$dir" list --status done --json
	detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
ids = sorted(item["id"] for item in items)
if ids != [1]:
    raise SystemExit(f"--status done 결과가 [1]이 아님: {ids}")
' "$dir/out")
	if [ -z "$detail" ]; then
		pass status-filter ""
	else
		fail status-filter "$detail"
	fi
fi

# -------------------------------------------------------------- tag filtering --

dir=$(workdir tag-filter)
run "$dir" add "집안일" --tag home
run "$dir" add "회사일" --tag work
run "$dir" list --tag home --json
detail=$(json_check '
import json, sys
items = json.loads(open(sys.argv[1], encoding="utf-8").read())
titles = sorted(item["title"] for item in items)
if titles != ["집안일"]:
    raise SystemExit(f"--tag home 결과가 집안일 하나가 아님: {titles}")
' "$dir/out")
if [ -z "$detail" ]; then
	pass tag-filter ""
else
	fail tag-filter "$detail"
fi

# ------------------------------------------------------------------ empty list --

dir=$(workdir empty-list)
run "$dir" list
status=$?
if [ $status -ne 0 ]; then
	fail empty-list "빈 목록인데 종료 코드가 $status"
elif [ -s "$dir/out" ] || [ -s "$dir/err" ]; then
	pass empty-list ""
else
	fail empty-list "빈 목록 안내 문구가 없음"
fi

# --------------------------------------------------------------- error cases --

dir=$(workdir unknown-id)
run "$dir" add "하나"
run "$dir" done 999
status=$?
if [ $status -eq 0 ]; then
	fail unknown-id "없는 id인데 종료 코드가 0"
elif [ -s "$dir/err" ]; then
	pass unknown-id "exit $status"
else
	fail unknown-id "종료 코드는 $status인데 stderr가 비어 있음"
fi

dir=$(workdir unknown-command)
run "$dir" definitely-not-a-command
status=$?
if [ $status -eq 0 ]; then
	fail unknown-command "모르는 하위 명령인데 종료 코드가 0"
else
	pass unknown-command "exit $status"
fi

dir=$(workdir bad-due)
run "$dir" add "날짜 이상" --due 2026-13-45
status=$?
if [ $status -eq 0 ]; then
	fail bad-due "잘못된 날짜인데 종료 코드가 0"
else
	pass bad-due "exit $status"
fi

# ------------------------------------------------------------------ file flag --

# Either placement is defensible from the request, so both are accepted.
dir=$(workdir file-flag)
target=$dir/custom/store.json
mkdir -p "$dir/custom"
run "$dir" add "다른 파일" --file "$target"
if [ ! -f "$target" ]; then
	run "$dir" --file "$target" add "다른 파일"
fi
if [ -f "$target" ]; then
	pass file-flag ""
else
	fail file-flag "--file 경로에 저장 파일이 생기지 않음: $(cat "$dir/err")"
fi

# --------------------------------------------------------------------- summary --

if [ "$failures" -eq 0 ]; then
	printf 'CHECK\t%s\tPASS\t%s\n' summary "$checks/$checks 통과"
	exit 0
fi
printf 'CHECK\t%s\tFAIL\t%s\n' summary "$failures/$checks 실패"
exit 1
