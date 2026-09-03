package cmd

import (
	"context"
	"fmt"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/urfave/cli/v3"
)

// initCommand — 보드를 초기화한다. 저장소당 한 번 (§1).
func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "보드를 초기화한다",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "hooks", Usage: "reference-transaction hook 설치"},
			&cli.BoolFlag{Name: "force", Usage: "기존 hook 덮어쓰기"},
			&cli.BoolFlag{Name: "no-agents-md", Usage: "에이전트 문서 생성 건너뛰기"},
		},
		Action: action(runInit),
	}
}

// doctorCommand — 환경을 점검한다 (§1).
func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:   "doctor",
		Usage:  "환경을 점검한다. FAIL 이 있으면 exit 1",
		Action: action(runDoctor),
	}
}

func runDoctor(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	id := b.Identity()
	ttl := b.ActivityTTL().String()
	info := map[string]any{
		"agent": id.Agent, "agent_source": id.AgentSource,
		"session": id.Session, "session_source": id.SessionSource,
		"liveness": "last_activity", "activity_ttl": ttl,
	}
	if x.json {
		return x.emit(info)
	}
	x.printf("agent      %s\n", id.Agent)
	x.printf("detected   %s\n", id.AgentSource)
	x.printf("session    %s\n", id.Session)
	x.printf("source     %s\n", id.SessionSource)
	x.printf("liveness   last_activity (%s threshold)  WARN\n", ttl)
	x.printf("hint       자동 회수가 느립니다. 훅을 설치하거나 release --mine 을 호출하세요.\n")
	return nil
}

// versionCommand — CLI/스키마/git 버전을 출력한다 (§1).
func versionCommand(v Version) *cli.Command {
	return &cli.Command{
		Name:  "version",
		Usage: "CLI·스키마·git 버전을 출력한다",
		Action: func(_ context.Context, c *cli.Command) error {
			x := newCtx(c)
			if x.json {
				return x.emit(map[string]string{"cli": v.CLI, "schema": v.Schema})
			}
			x.printf("ppwk    %s\nschema  %s\n", v.CLI, v.Schema)
			return nil
		},
	}
}

// runInit 은 보드를 초기화한다.
func runInit(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	result, err := b.Init(board.InitOptions{
		Hooks:      x.cmd.Bool("hooks"),
		NoAgentsMD: x.cmd.Bool("no-agents-md"),
		Force:      x.cmd.Bool("force"),
	})
	if err != nil {
		return err
	}
	if x.json {
		return x.emit(result)
	}

	if result.SchemaCreated {
		x.printf("보드를 초기화했습니다.\n")
	} else {
		x.printf("이미 초기화된 보드입니다.\n")
	}
	for _, doc := range result.DocsCreated {
		x.printf("  생성  %s\n", doc)
	}
	for _, w := range result.Warnings {
		fmt.Fprintf(x.stderr, "경고: %s\n", w)
	}
	for _, n := range result.Notes {
		x.printf("\n%s\n", n)
	}
	return nil
}
