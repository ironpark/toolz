package cmd

import (
	"context"
	"io"
	"time"

	"github.com/urfave/cli/v3"
)

// Version 정보는 main 에서 주입한다.
type Version struct {
	CLI    string
	Schema string
}

const defaultTimeout = 10 * time.Second

// New 는 ppwk 루트 명령을 만든다.
func New(v Version, stdout, stderr io.Writer) *cli.Command {
	root := &cli.Command{
		Name:                  "ppwk",
		Usage:                 "git ref 기반 이슈 보드",
		Version:               v.CLI,
		Writer:                stdout,
		ErrWriter:             stderr,
		EnableShellCompletion: true,
		HideHelpCommand:       true,
		Flags:                 globalFlags(),
		Commands: []*cli.Command{
			// 1. 저장소 관리
			initCommand(),
			doctorCommand(),
			versionCommand(v),

			// 2. 이슈 생성과 조회
			addCommand(),
			listCommand(),
			showCommand(),
			historyCommand(),
			editCommand(),

			// 3. 상태 전이
			claimCommand(),
			startCommand(),
			doneCommand(),
			blockCommand(),
			unblockCommand(),
			releaseCommand(),
			cancelCommand(),

			// 4. 스케줄링
			nextCommand(),
			reapCommand(),
			agentsCommand(),
			internalCommand(),

			// 5. plan 과 phase
			planCommand(),

			// 5.5 결정 기록
			decideCommand(),
			decisionsCommand(),

			// 6. 변경 감지
			watchCommand(),
			hookCommand(),

			// 7. 운영
			//
			// import 와 gc 는 두지 않는다. 백업·복원은 git bundle 이 이력까지
			// 보존하며 정확히 하고, 정리는 git gc 가 이미 한다. 얇게 감싸면
			// 우리가 더하는 것 없이 표면만 늘어난다.
			exportCommand(),
			fsckCommand(),
			archiveCommand(),
			unarchiveCommand(),
		},
	}

	// 종료는 main 이 결정한다. 라이브러리가 os.Exit 하지 않도록 막는다.
	root.ExitErrHandler = func(context.Context, *cli.Command, error) {}

	// 잘못된 사용법은 exit 2 로 통일한다 (§0.3).
	root.OnUsageError = func(_ context.Context, _ *cli.Command, err error, _ bool) error {
		return UsageError("%v", err)
	}
	root.CommandNotFound = func(_ context.Context, c *cli.Command, name string) {
		c.ErrWriter.Write([]byte("알 수 없는 명령: " + name + "\n"))
	}
	return root
}

// globalFlags 는 모든 명령에 적용되는 플래그다 (§0.1).
func globalFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:  "json",
			Usage: "출력을 JSON 으로. 스크립트/에이전트용",
		},
		&cli.StringFlag{
			Name:  "agent",
			Usage: "에이전트 신원 override",
			// PPWK_AGENT 를 여기 묶지 않는다. 묶으면 환경변수로 온 값이
			// 플래그로 온 값과 구분되지 않아 doctor 가 감지 근거를 틀리게
			// 말한다 (T4.27). 결정 순서는 session.Resolve 하나가 갖는다.
		},
		&cli.StringFlag{
			Name:  "C",
			Usage: "저장소 경로",
			Value: ".",
		},
		&cli.BoolFlag{
			Name:    "quiet",
			Aliases: []string{"q"},
			Usage:   "오류만 출력",
		},
		&cli.BoolFlag{
			Name:    "verbose",
			Aliases: []string{"v"},
			Usage:   "git 명령 실행 로그 포함",
		},
		&cli.BoolFlag{
			Name:    "no-color",
			Usage:   "색상 비활성화. TTY 가 아니면 자동으로 켜짐",
			Sources: cli.EnvVars("NO_COLOR"),
		},
		&cli.DurationFlag{
			Name:  "timeout",
			Usage: "CAS 재시도 총 상한",
			Value: defaultTimeout,
		},
	}
}
