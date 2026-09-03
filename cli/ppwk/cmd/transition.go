package cmd

import (
	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/urfave/cli/v3"
)

// transitionFlags 는 상태 전이 명령의 공통 플래그다 (§3).
func transitionFlags(extra ...cli.Flag) []cli.Flag {
	flags := []cli.Flag{
		&cli.BoolFlag{Name: "allow-shared-worktree", Usage: "worktree 배타 확보를 건너뜀"},
		&cli.StringFlag{Name: "message", Usage: "이벤트 subject 에 붙일 사유"},
		&cli.IntFlag{Name: "retry", Usage: "CAS 실패 시 재시도 횟수. 기본 0 — 즉시 exit 4"},
	}
	return append(flags, extra...)
}

// claimCommand — open → claimed. 예약만 하고 시작은 나중에 (§3).
func claimCommand() *cli.Command {
	return &cli.Command{
		Name:      "claim",
		Usage:     "이슈를 예약한다 (open → claimed)",
		ArgsUsage: "<id>",
		Flags:     transitionFlags(),
		Action:    action(runTransition(board.ActionClaim)),
	}
}

// startCommand — open → working, claimed → working. open 이면 claim 을 겸한다 (D16).
func startCommand() *cli.Command {
	return &cli.Command{
		Name:      "start",
		Usage:     "작업을 시작한다 (open|claimed → working)",
		ArgsUsage: "<id>",
		Flags:     transitionFlags(),
		Action:    action(runTransition(board.ActionStart)),
	}
}

// doneCommand — working → done. archive 로 이동한다 (§3).
func doneCommand() *cli.Command {
	return &cli.Command{
		Name:      "done",
		Usage:     "작업을 완료한다 (working → done)",
		ArgsUsage: "<id>",
		Flags:     transitionFlags(),
		Action:    action(runTransition(board.ActionDone)),
	}
}

// blockCommand — working → blocked (§3).
func blockCommand() *cli.Command {
	return &cli.Command{
		Name:      "block",
		Usage:     "작업을 차단 상태로 둔다 (working → blocked)",
		ArgsUsage: "<id>",
		Flags: transitionFlags(
			&cli.StringFlag{Name: "on", Usage: "차단 원인 이슈 ID"},
		),
		Action: action(runTransition(board.ActionBlock)),
	}
}

// unblockCommand — blocked → working (§3).
func unblockCommand() *cli.Command {
	return &cli.Command{
		Name:      "unblock",
		Usage:     "차단을 해제한다 (blocked → working)",
		ArgsUsage: "<id>",
		Flags:     transitionFlags(),
		Action:    action(runTransition(board.ActionUnblock)),
	}
}

// releaseCommand — claimed → open. 소유권 반납 (§3).
func releaseCommand() *cli.Command {
	return &cli.Command{
		Name:      "release",
		Usage:     "소유권을 반납한다 (claimed → open)",
		ArgsUsage: "[id]", // --mine 이면 생략한다
		Flags: transitionFlags(
			&cli.BoolFlag{Name: "force", Usage: "소유자가 아니어도 강제"},
			&cli.BoolFlag{Name: "mine", Usage: "현재 세션이 보유한 이슈 전체에 적용"},
		),
		Action: action(runTransition(board.ActionRelease)),
	}
}

// cancelCommand — any → cancelled. archive 로 이동한다 (§3).
func cancelCommand() *cli.Command {
	return &cli.Command{
		Name:      "cancel",
		Usage:     "이슈를 취소한다 (any → cancelled)",
		ArgsUsage: "<id>",
		Flags: transitionFlags(
			&cli.BoolFlag{Name: "force", Usage: "소유자가 아니어도 강제"},
		),
		Action: action(runTransition(board.ActionCancel)),
	}
}
