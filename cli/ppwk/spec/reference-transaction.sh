#!/bin/sh
#
# ppwk reference-transaction hook
#
# Install to $GIT_COMMON_DIR/hooks/reference-transaction (chmod +x).
# Runs INSIDE the git process on every ref transaction, so it must:
#   - exit immediately for stages/refs it does not care about
#   - never write a ref (recursion)
#   - never block (fire-and-forget to a socket, hard timeout)
#
# stdin lines: <old-oid> <new-oid> <ref-name>
# argv[1]    : preparing | prepared | committed | aborted

STAGE="$1"

# Only act once the transaction is durable.
[ "$STAGE" = "committed" ] || exit 0

PREFIX="refs/ppwk/"
SOCK="${PPWK_SOCK:-${GIT_COMMON_DIR:-.git}/ppwk.sock}"

# Never let a broken listener wedge a git command.
EMIT_TIMEOUT="${PPWK_EMIT_TIMEOUT:-0.2}"

payload=""
while read -r old new ref; do
	case "$ref" in
	"$PREFIX"*) ;;
	*) continue ;;
	esac

	case "$new" in
	0000000000000000000000000000000000000000 | \
		0000000000000000000000000000000000000000000000000000000000000000)
		kind="deleted"
		;;
	*)
		case "$old" in
		0000000000000000000000000000000000000000 | \
			0000000000000000000000000000000000000000000000000000000000000000)
			kind="created"
			;;
		*)
			kind="updated"
			;;
		esac
		;;
	esac

	payload="${payload}{\"ref\":\"${ref}\",\"old\":\"${old}\",\"new\":\"${new}\",\"kind\":\"${kind}\"}
"
done

# Nothing in our namespace: the overwhelmingly common case (normal commits,
# fetch, rebase, checkout). Exit before touching the filesystem.
[ -n "$payload" ] || exit 0

if [ -S "$SOCK" ] && command -v socat >/dev/null 2>&1; then
	printf '%s' "$payload" |
		timeout "$EMIT_TIMEOUT" socat - "UNIX-CONNECT:$SOCK" >/dev/null 2>&1 || true
elif [ -p "$SOCK" ]; then
	# FIFO fallback: O_NONBLOCK write, drops if no reader.
	timeout "$EMIT_TIMEOUT" sh -c "printf '%s' \"\$1\" >>\"$SOCK\"" _ "$payload" >/dev/null 2>&1 || true
fi

exit 0
