#!/usr/bin/env bash
# Acceptance checks for the codex-kvstore fixture.
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
# The command surface is five subcommands, but the properties underneath them --
# durability across processes, recovery from a truncated tail, atomic
# compaction, and concurrent writers -- cut across the whole implementation.
# Each check runs in its own directory so one broken behaviour cannot cascade.

set -u

WORKSPACE=${PLANR_EVAL_WORKSPACE:?PLANR_EVAL_WORKSPACE must point at the agent workspace}
SCRATCH=$PWD
BIN=$SCRATCH/kv-under-test
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

ALL_CHECKS="set-get list-sorted del-removes missing-key deleted-key del-missing \
empty-list value-with-spaces persistence file-flag truncated-tail garbage-tail \
compact-preserves compact-shrinks compact-atomic concurrent-writers \
unknown-command missing-arguments go-test readme"

workdir() {
	local dir=$SCRATCH/case-$1
	rm -rf "$dir"
	mkdir -p "$dir"
	printf '%s' "$dir"
}

# Run the built CLI inside a case directory, capturing streams separately so a
# check can tell "printed an error" from "printed a result".
run() {
	local dir=$1
	shift
	(cd "$dir" && "$BIN" "$@" >"$dir/out" 2>"$dir/err")
	printf '%s' $? >"$dir/status"
}

out() { cat "$1/out"; }
status() { cat "$1/status"; }

# ---------------------------------------------------------------- go module --

if [ -f "$WORKSPACE/go.mod" ]; then
	module=$(sed -n 's/^module[[:space:]][[:space:]]*//p' "$WORKSPACE/go.mod" | head -1)
	if [ "$module" = "example.com/kv" ]; then
		pass go-module "module $module"
	else
		fail go-module "module 이름이 example.com/kv가 아님: ${module:-없음}"
	fi
else
	fail go-module "go.mod 없음"
fi

# -------------------------------------------------------- no dependencies --

if [ -f "$WORKSPACE/go.mod" ] && grep -qE '^[[:space:]]*require' "$WORKSPACE/go.mod" 2>/dev/null; then
	fail no-dependencies "go.mod에 require 블록이 있음"
else
	pass no-dependencies ""
fi

# -------------------------------------------------------------------- build --

build_output=$(cd "$WORKSPACE" && go build -o "$BIN" . 2>&1)
if [ $? -eq 0 ] && [ -x "$BIN" ]; then
	pass build ""
else
	fail build "$build_output"
	skip_rest "빌드 실패" $ALL_CHECKS
	printf 'CHECK\t%s\tFAIL\t%s\n' summary "$failures/$checks 실패"
	exit 1
fi

# ------------------------------------------------------------------- basics --

dir=$(workdir basics)
run "$dir" set alpha one
run "$dir" set beta two
run "$dir" get alpha
if [ "$(out "$dir")" = "one" ] && [ "$(status "$dir")" -eq 0 ]; then
	pass set-get ""
else
	fail set-get "get alpha = [$(out "$dir")] status=$(status "$dir") err=$(cat "$dir/err")"
fi

run "$dir" set gamma three
run "$dir" list
if [ "$(out "$dir")" = "$(printf 'alpha\tone\nbeta\ttwo\ngamma\tthree')" ]; then
	pass list-sorted ""
else
	fail list-sorted "list = [$(out "$dir")]"
fi

run "$dir" del beta
if [ "$(status "$dir")" -ne 0 ]; then
	fail del-removes "del beta가 실패함: $(cat "$dir/err")"
else
	run "$dir" list
	if [ "$(out "$dir")" = "$(printf 'alpha\tone\ngamma\tthree')" ]; then
		pass del-removes ""
	else
		fail del-removes "del 후 list = [$(out "$dir")]"
	fi
fi

# --------------------------------------------------------------- exit codes --

run "$dir" get nosuchkey
if [ "$(status "$dir")" -ne 0 ] && [ -s "$dir/err" ]; then
	pass missing-key "status=$(status "$dir")"
else
	fail missing-key "없는 키 get이 status=$(status "$dir"), stderr=[$(cat "$dir/err")]"
fi

run "$dir" get beta
if [ "$(status "$dir")" -ne 0 ]; then
	pass deleted-key "status=$(status "$dir")"
else
	fail deleted-key "지운 키 get이 성공함: [$(out "$dir")]"
fi

run "$dir" del nosuchkey
if [ "$(status "$dir")" -ne 0 ] && [ -s "$dir/err" ]; then
	pass del-missing "status=$(status "$dir")"
else
	fail del-missing "없는 키 del이 status=$(status "$dir")"
fi

dir=$(workdir empty)
run "$dir" list
if [ "$(status "$dir")" -eq 0 ] && [ -z "$(out "$dir")" ]; then
	pass empty-list ""
else
	fail empty-list "빈 저장소 list가 status=$(status "$dir") out=[$(out "$dir")]"
fi

dir=$(workdir spaces)
run "$dir" set title "hello there world"
run "$dir" get title
if [ "$(out "$dir")" = "hello there world" ]; then
	pass value-with-spaces ""
else
	fail value-with-spaces "공백 포함 값 = [$(out "$dir")]"
fi

# -------------------------------------------------------------- persistence --
#
# Every invocation is a separate process, so this also proves the log is read
# back rather than kept in memory.

dir=$(workdir persistence)
run "$dir" set k1 v1
run "$dir" set k2 v2
run "$dir" set k1 v1-updated
run "$dir" get k1
if [ "$(out "$dir")" = "v1-updated" ]; then
	pass persistence ""
else
	fail persistence "덮어쓴 값이 유지되지 않음: [$(out "$dir")]"
fi

dir=$(workdir file-flag)
target=$dir/nested/store.log
mkdir -p "$dir/nested"
run "$dir" set --file "$target" k v
if [ ! -f "$target" ]; then
	run "$dir" --file "$target" set k v
fi
if [ -f "$target" ]; then
	pass file-flag ""
else
	fail file-flag "--file 경로에 로그가 생기지 않음: $(cat "$dir/err")"
fi

# ------------------------------------------------------------ crash recovery --
#
# A killed script leaves a partial final record. Everything written before it
# must still load; only the interrupted write may be lost.

dir=$(workdir truncated)
run "$dir" set a 1
run "$dir" set b 2
run "$dir" set c 3
log=$(ls "$dir"/*.log 2>/dev/null | head -1)
if [ -z "$log" ]; then
	log=$dir/kv.log
fi
if [ ! -f "$log" ]; then
	fail truncated-tail "로그 파일을 찾지 못함"
	fail garbage-tail "로그 파일을 찾지 못함"
else
	size=$(wc -c <"$log" | tr -d ' ')
	cp "$log" "$dir/backup.log"
	# Chop the last few bytes so the final record is incomplete.
	truncated=$((size - 3))
	dd if="$dir/backup.log" of="$log" bs=1 count=$truncated 2>/dev/null
	run "$dir" get a
	if [ "$(out "$dir")" = "1" ] && [ "$(status "$dir")" -eq 0 ]; then
		pass truncated-tail "앞선 레코드 유지됨"
	else
		fail truncated-tail "잘린 로그에서 a를 읽지 못함: status=$(status "$dir") out=[$(out "$dir")] err=$(cat "$dir/err")"
	fi

	cp "$dir/backup.log" "$log"
	printf 'this is not a valid record' >>"$log"
	run "$dir" get b
	if [ "$(out "$dir")" = "2" ] && [ "$(status "$dir")" -eq 0 ]; then
		pass garbage-tail "쓰레기 꼬리를 무시함"
	else
		fail garbage-tail "쓰레기가 붙은 로그에서 b를 읽지 못함: status=$(status "$dir") err=$(cat "$dir/err")"
	fi
fi

# ------------------------------------------------------------------ compact --

dir=$(workdir compact)
i=0
while [ $i -lt 40 ]; do
	run "$dir" set churn "value-$i"
	i=$((i + 1))
done
run "$dir" set keep permanent
run "$dir" set doomed temporary
run "$dir" del doomed
log=$(ls "$dir"/*.log 2>/dev/null | head -1)
before=$(wc -c <"$log" 2>/dev/null | tr -d ' ')
run "$dir" compact
compact_status=$(status "$dir")
run "$dir" list
if [ "$(out "$dir")" = "$(printf 'churn\tvalue-39\nkeep\tpermanent')" ]; then
	pass compact-preserves ""
else
	fail compact-preserves "compact 후 list = [$(out "$dir")] (compact status=$compact_status)"
fi

after=$(wc -c <"$log" 2>/dev/null | tr -d ' ')
if [ -n "$before" ] && [ -n "$after" ] && [ "$after" -lt "$before" ]; then
	pass compact-shrinks "$before -> $after bytes"
else
	fail compact-shrinks "로그가 줄지 않음: ${before:-?} -> ${after:-?} bytes"
fi

# The store must still be readable by a fresh process after compaction, and the
# compaction must not have left a stray temporary file behind as the live log.
run "$dir" get keep
strays=$(ls -a "$dir" | grep -c -E '\.tmp$|\.log\.[0-9]+$|~$' || true)
if [ "$(out "$dir")" = "permanent" ] && [ "$strays" -eq 0 ]; then
	pass compact-atomic ""
else
	fail compact-atomic "compact 후 get keep=[$(out "$dir")], 임시 파일 $strays개 남음"
fi

# ------------------------------------------------------------- concurrency --
#
# Twenty parallel writers, each its own process. Anything less than twenty keys
# afterwards means writes were lost or the log was corrupted.

dir=$(workdir concurrent)
# Seed the store first. A read-modify-write implementation is only visibly racy
# when the write is slow enough for two processes to overlap, and with an empty
# store the window is so narrow that a broken implementation can pass by luck.
i=0
while [ $i -lt 200 ]; do
	run "$dir" set "seed$i" "seed-value-$i"
	i=$((i + 1))
done

lost=""
round=1
while [ $round -le 3 ]; do
	i=0
	while [ $i -lt 30 ]; do
		(cd "$dir" && "$BIN" set "r${round}key$i" "value$i" >/dev/null 2>&1) &
		i=$((i + 1))
	done
	wait
	run "$dir" list
	present=$(out "$dir" | grep -c "^r${round}key" || true)
	if [ "$present" -ne 30 ]; then
		lost="$lost round$round=$present/30"
	fi
	round=$((round + 1))
done

run "$dir" list
seeds=$(out "$dir" | grep -c '^seed' || true)
if [ -z "$lost" ] && [ "$seeds" -eq 200 ]; then
	pass concurrent-writers "3x30 병렬 쓰기, seed 200개 유지"
else
	fail concurrent-writers "병렬 쓰기 유실: ${lost:-없음}, seed $seeds/200"
fi

# ---------------------------------------------------------------- CLI errors --

dir=$(workdir cli-errors)
run "$dir" bogus-subcommand
if [ "$(status "$dir")" -ne 0 ] && [ -s "$dir/err" ]; then
	pass unknown-command "status=$(status "$dir")"
else
	fail unknown-command "모르는 하위 명령이 status=$(status "$dir")"
fi

run "$dir" set onlykey
if [ "$(status "$dir")" -ne 0 ] && [ -s "$dir/err" ]; then
	pass missing-arguments "status=$(status "$dir")"
else
	fail missing-arguments "인자 부족이 status=$(status "$dir")"
fi

# ----------------------------------------------------------------- go test --

test_output=$(cd "$WORKSPACE" && go test ./... 2>&1)
if [ $? -eq 0 ]; then
	if printf '%s' "$test_output" | grep -q 'no test files'; then
		fail go-test "테스트 파일이 없음"
	else
		pass go-test ""
	fi
else
	fail go-test "$test_output"
fi

# ------------------------------------------------------------------- readme --

if [ -f "$WORKSPACE/README.md" ] && grep -q 'kv ' "$WORKSPACE/README.md"; then
	pass readme ""
else
	fail readme "README.md에 kv 사용 예시가 없음"
fi

# ------------------------------------------------------------------ summary --

if [ "$failures" -eq 0 ]; then
	printf 'CHECK\t%s\tPASS\t%s\n' summary "$checks/$checks 통과"
	exit 0
fi
printf 'CHECK\t%s\tFAIL\t%s\n' summary "$failures/$checks 실패"
exit 1
