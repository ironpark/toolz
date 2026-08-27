#!/usr/bin/env bash
# Acceptance checks for the codex-regex fixture.
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
# The command surface is tiny on purpose; nearly every check here targets the
# pattern engine instead, which is where the work actually is.

set -u

WORKSPACE=${PLANR_EVAL_WORKSPACE:?PLANR_EVAL_WORKSPACE must point at the agent workspace}
SCRATCH=$PWD
BIN=$SCRATCH/rx-under-test
SAMPLE=$SCRATCH/sample.txt
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

ALL_PATTERN_CHECKS="literal dot star plus question class class-range class-negate \
anchor-start anchor-end alternation group group-repeat escape-dot escape-backslash \
combined greedy-leftmost only-flag count-flag stdin no-match-exit match-exit \
bad-group bad-class bad-trailing-escape bad-quantifier missing-file"

# Run the built CLI, capturing stdout and stderr separately so checks can tell
# "printed an error" from "printed a result".
run() {
	"$BIN" "$@" >"$SCRATCH/out" 2>"$SCRATCH/err"
	printf '%s' $? >"$SCRATCH/status"
}

status() { cat "$SCRATCH/status"; }
out() { cat "$SCRATCH/out"; }

# expect_lines <name> <expected stdout> <args...>
expect_lines() {
	local name=$1 expected=$2
	shift 2
	run "$@"
	if [ "$(out)" = "$expected" ]; then
		pass "$name" ""
	else
		fail "$name" "want [$expected] got [$(out)] status=$(status) err=$(cat "$SCRATCH/err")"
	fi
}

# expect_error <name> <args...> -- an invalid input must be reported, not
# silently treated as a literal pattern.
expect_error() {
	local name=$1
	shift
	run "$@"
	if [ "$(status)" -eq 0 ]; then
		fail "$name" "종료 코드 0으로 성공함: out=[$(out)]"
	elif [ ! -s "$SCRATCH/err" ]; then
		fail "$name" "stderr가 비어 있음 (status=$(status))"
	else
		pass "$name" "status=$(status)"
	fi
}

# ---------------------------------------------------------------- go module --

if [ -f "$WORKSPACE/go.mod" ]; then
	module=$(sed -n 's/^module[[:space:]][[:space:]]*//p' "$WORKSPACE/go.mod" | head -1)
	if [ "$module" = "example.com/rx" ]; then
		pass go-module "module $module"
	else
		fail go-module "module 이름이 example.com/rx가 아님: ${module:-없음}"
	fi
else
	fail go-module "go.mod 없음"
fi

# --------------------------------------------------- own matcher, not regexp --
#
# The whole point of the request is the matching engine, so importing the
# standard library's regexp is a failure even when every behavioural check
# passes. Comments and strings are stripped first so a mention in prose does
# not count against the run.

regexp_hits=$(cd "$WORKSPACE" && grep -rn --include='*.go' -E '^[[:space:]]*(_[[:space:]]+)?"regexp(/syntax)?"' . 2>/dev/null | head -5)
if [ -n "$regexp_hits" ]; then
	fail own-matcher "regexp를 import함: $regexp_hits"
else
	pass own-matcher ""
fi

# ------------------------------------------------------- no dependencies --

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
	skip_rest "빌드 실패" $ALL_PATTERN_CHECKS go-test readme
	printf 'CHECK\t%s\tFAIL\t%s\n' summary "$failures/$checks 실패"
	exit 1
fi

# ------------------------------------------------------------------- sample --

cat >"$SAMPLE" <<'SAMPLE'
alpha
beta
gamma
abc
aaa
abab
a.b
a\b
xyz
foo123
SAMPLE

# ------------------------------------------------------------------ syntax --

expect_lines literal 'alpha' 'alpha' "$SAMPLE"
expect_lines dot 'abc' 'a.c' "$SAMPLE"
expect_lines star "$(printf 'alpha\nabc\naaa\nabab\na.b\na\\b')" '^a.*' "$SAMPLE"
expect_lines plus 'aaa' '^a+$' "$SAMPLE"
expect_lines question "$(printf 'abc\nabab')" '^ab?a?b' "$SAMPLE"
expect_lines class "$(printf 'alpha\nbeta\nabc\naaa\nabab\na.b\na\\b')" '^[ab]' "$SAMPLE"
expect_lines class-range 'foo123' '[0-9]' "$SAMPLE"
expect_lines class-negate 'foo123' '^[^a-z]*[a-z]*[0-9]' "$SAMPLE"
expect_lines anchor-start 'xyz' '^x' "$SAMPLE"
expect_lines anchor-end 'gamma' 'mma$' "$SAMPLE"
expect_lines alternation "$(printf 'beta\ngamma')" 'beta|gamma' "$SAMPLE"
expect_lines group 'abc' '^a(b|d)c$' "$SAMPLE"
expect_lines group-repeat 'abab' '^(ab)+$' "$SAMPLE"
expect_lines escape-dot 'a.b' '^a\.b$' "$SAMPLE"
expect_lines escape-backslash 'a\b' '^a\\b$' "$SAMPLE"
expect_lines combined "$(printf 'abc\naaa\nabab')" '^(a|b)+c?$|^abab$' "$SAMPLE"

# ------------------------------------------------------- greedy and leftmost --
#
# `a+` against `aaa` is one greedy match, not three single-character ones.

run -o '^a+' "$SAMPLE"
if [ "$(out)" = "$(printf 'a\na\naaa\na\na\na')" ] || [ "$(out)" = "$(printf 'a\na\naaa\nab\na\na')" ]; then
	pass greedy-leftmost "matched greedily"
elif printf '%s' "$(out)" | grep -qx 'aaa'; then
	pass greedy-leftmost "aaa matched as one run"
else
	fail greedy-leftmost "aaa가 하나의 매치로 나오지 않음: [$(out)]"
fi

# -------------------------------------------------------------------- flags --

# Printing every match on a line or only the leftmost one are both defensible
# readings of the request, so the check only requires that what is printed is
# the matched text and nothing else.
run -o 'b' "$SAMPLE"
if [ -n "$(out)" ] && [ -z "$(out | grep -vx 'b')" ]; then
	pass only-flag "$(out | wc -l | tr -d ' ') lines"
else
	fail only-flag "-o가 매치된 부분만 출력하지 않음: [$(out)]"
fi

expect_lines count-flag '2' -c '^ab' "$SAMPLE"

printf 'hello\nworld\n' | "$BIN" 'wor' >"$SCRATCH/out" 2>"$SCRATCH/err"
if [ "$(out)" = "world" ]; then
	pass stdin ""
else
	fail stdin "표준 입력을 읽지 않음: [$(out)] err=[$(cat "$SCRATCH/err")]"
fi

# ---------------------------------------------------------------- exit codes --

run 'zzzzz-nothing-matches' "$SAMPLE"
if [ "$(status)" -eq 1 ] && [ -z "$(out)" ]; then
	pass no-match-exit "status=1, 출력 없음"
else
	fail no-match-exit "매치 없음이 status=1/무출력이 아님: status=$(status) out=[$(out)]"
fi

run 'alpha' "$SAMPLE"
if [ "$(status)" -eq 0 ]; then
	pass match-exit ""
else
	fail match-exit "매치가 있는데 status=$(status)"
fi

# ------------------------------------------------------------------- errors --

expect_error bad-group 'a(b' "$SAMPLE"
expect_error bad-class '[z-a]' "$SAMPLE"
expect_error bad-trailing-escape 'abc\' "$SAMPLE"
expect_error bad-quantifier '*abc' "$SAMPLE"
expect_error missing-file 'alpha' "$SCRATCH/does-not-exist.txt"

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

if [ -f "$WORKSPACE/README.md" ] && grep -q 'rx ' "$WORKSPACE/README.md"; then
	pass readme ""
else
	fail readme "README.md에 rx 사용 예시가 없음"
fi

# ------------------------------------------------------------------ summary --

if [ "$failures" -eq 0 ]; then
	printf 'CHECK\t%s\tPASS\t%s\n' summary "$checks/$checks 통과"
	exit 0
fi
printf 'CHECK\t%s\tFAIL\t%s\n' summary "$failures/$checks 실패"
exit 1
