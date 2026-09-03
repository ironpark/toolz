#!/bin/sh
#
# ppwk-watch — install the hook, then listen for issue changes.
#
#   ppwk-watch install    # set up hook + config
#   ppwk-watch listen     # print change events as they happen
#
# The listener is the ONLY long-lived process. The hook itself stays dumb.

set -e

COMMON="$(git rev-parse --path-format=absolute --git-common-dir)"
SOCK="$COMMON/ppwk.sock"

cmd_install() {
	hp="$(git config --get core.hooksPath || true)"
	if [ -n "$hp" ]; then
		echo "warning: core.hooksPath is set to '$hp'." >&2
		echo "         Install the hook there instead of $COMMON/hooks." >&2
		dest="$hp"
	else
		dest="$COMMON/hooks"
	fi

	mkdir -p "$dest"
	if [ -e "$dest/reference-transaction" ]; then
		echo "error: $dest/reference-transaction already exists; merge manually." >&2
		exit 1
	fi
	cp "$(dirname "$0")/reference-transaction" "$dest/reference-transaction"
	chmod +x "$dest/reference-transaction"

	# Keep ppwk refs out of `git log` decoration noise.
	git config --add log.excludeDecoration refs/ppwk/

	echo "installed: $dest/reference-transaction"
	echo "socket:    $SOCK"
}

cmd_listen() {
	if ! command -v socat >/dev/null 2>&1; then
		echo "socat not found; falling back to polling." >&2
		exec "$0" poll
	fi
	rm -f "$SOCK"
	trap 'rm -f "$SOCK"' EXIT INT TERM
	socat "UNIX-LISTEN:$SOCK,fork,mode=600" -
}

# Polling fallback. Correct even when the hook is absent, gc has packed the
# refs, or the reftable backend is in use — none of which expose ref files.
cmd_poll() {
	interval="${PPWK_POLL_INTERVAL:-2}"
	prev=""
	while :; do
		cur="$(git for-each-ref --format='%(refname) %(objectname)' refs/ppwk/)"
		if [ "$cur" != "$prev" ] && [ -n "$prev" ]; then
			printf '%s\n' "$cur" | diff - <(printf '%s\n' "$prev") >/dev/null 2>&1 || {
				printf '%s\n' "$cur" | comm -23 - <(printf '%s\n' "$prev" | sort) 2>/dev/null ||
					printf '%s\n' "$cur"
			}
		fi
		prev="$cur"
		sleep "$interval"
	done
}

case "${1:-}" in
install) cmd_install ;;
listen) cmd_listen ;;
poll) cmd_poll ;;
*)
	echo "usage: $0 {install|listen|poll}" >&2
	exit 2
	;;
esac
