package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
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

// check 는 doctor 항목 하나다 (features §1).
//
// 표시와 판정을 한 값에 모아 둔다. 항목마다 printf 를 늘리면 사람이 보는
// 출력과 --json 이 갈라지고, FAIL 집계가 출력 코드에 숨는다.
type check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // OK / WARN / FAIL
	Value  string `json:"value"`
	Via    string `json:"via,omitempty"`
	Hint   string `json:"hint,omitempty"`
}

const (
	statusOK   = "OK"
	statusWarn = "WARN"
	statusFail = "FAIL"
)

// runDoctor 는 항목을 모아 보고한다. FAIL 이 하나라도 있으면 exit 1 (§1).
func runDoctor(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	checks := doctorChecks(b)

	if x.json {
		if err := x.emit(map[string]any{"checks": checks}); err != nil {
			return err
		}
	} else {
		rows := make([][]string, 0, len(checks))
		for _, c := range checks {
			via := c.Via
			if via != "" {
				via = "via " + via
			}
			rows = append(rows, []string{c.Name, c.Status, c.Value, via})
		}
		if err := x.table(rows); err != nil {
			return err
		}
		for _, c := range checks {
			if c.Hint != "" {
				x.printf("hint  %s: %s\n", c.Name, c.Hint)
			}
		}
	}

	for _, c := range checks {
		if c.Status == statusFail {
			return &Error{Code: ExitGeneral, Kind: "doctor_fail", Msg: "점검 항목에 FAIL 이 있습니다"}
		}
	}
	return nil
}

func doctorChecks(b *board.Board) []check {
	id := b.Identity()
	checks := []check{
		// 감지 실패는 오류가 아니라 정보다 (§1). 폴백도 OK 로 보고한다.
		{Name: "agent id", Status: statusOK, Value: id.Agent, Via: id.AgentSource},
		{Name: "session id", Status: statusOK, Value: id.Session, Via: id.SessionSource},
	}
	checks = append(checks, lockingCheck(b), worktreeCheck(b), livenessCheck(b), holdingCheck(b))
	return checks
}

// lockingCheck 는 flock 이 실제로 동작하는지 시도해 본다 (design §719).
func lockingCheck(b *board.Board) check {
	if err := b.ProbeLock(); err != nil {
		return check{Name: "file locking", Status: statusFail, Value: err.Error(),
			Hint: "flock 을 지원하지 않는 파일시스템(NFS 등)으로 보입니다. 로컬 디스크에 저장소를 두세요."}
	}
	return check{Name: "file locking", Status: statusOK, Value: "flock 동작"}
}

// worktreeCheck 는 이 worktree 를 누가 쥐고 있는지 본다 (§3.6 worktree 배타).
func worktreeCheck(b *board.Board) check {
	c := check{Name: "worktree", Status: statusOK, Value: b.Root()}
	lease, ok := b.WorktreeLease()
	switch {
	case !ok:
		c.Via = "미등록 — 첫 상태 변경 시 확보"
	case lease.Session == b.Identity().Session:
		c.Via = "배타 확보"
	case b.LeaseAlive(lease):
		c.Status = statusWarn
		c.Via = fmt.Sprintf("%s (session %s) 가 점유 중", lease.Agent, lease.Session)
		c.Hint = "git worktree add 로 새 worktree 를 만드세요. 의도한 구성이라면 --allow-shared-worktree 또는 git config ppwk.allowSharedWorktree true 를 쓰세요."
	default:
		c.Via = fmt.Sprintf("%s 의 죽은 기록이 남아 있음", lease.Agent)
	}
	return c
}

// livenessCheck 는 무엇으로 죽음을 판정하는지 보고한다 (§3.6).
//
// 훅이 있으면 즉시 감지이고, 없으면 임계값을 기다려야 하므로 WARN 이다.
// 훅이 들어오면 이 항목은 저절로 OK 가 된다 — 조건 없이 WARN 을 박아 두면
// 구현이 끝난 뒤에도 경고가 남는다.
func livenessCheck(b *board.Board) check {
	if lease, ok := b.WorktreeLease(); ok && lease.HookPID != nil {
		return check{Name: "liveness", Status: statusOK,
			Value: fmt.Sprintf("hook_pid %d", *lease.HookPID), Via: "즉시 감지"}
	}
	return check{Name: "liveness", Status: statusWarn,
		Value: "last_activity " + b.ActivityTTL().String(), Via: "훅 없음",
		Hint: "자동 회수가 느립니다. 훅을 설치하거나 세션 종료 시 release --mine 을 호출하세요."}
}

// holdingCheck 는 이 세션이 쥐고 있는 이슈다 (§1).
func holdingCheck(b *board.Board) check {
	entries, err := b.List(board.ListOptions{
		Status: []model.Status{model.StatusClaimed, model.StatusWorking},
		Owner:  b.Identity().Agent,
	})
	if err != nil {
		return check{Name: "holding", Status: statusFail, Value: err.Error()}
	}
	var ids []string
	for _, entry := range entries {
		// owner 이름만 맞고 session 이 다르면 죽은 세션의 것이다 (§3.6).
		issue, err := b.Show(entry.ID)
		if err != nil || issue.Session != b.Identity().Session {
			continue
		}
		ids = append(ids, entry.ID)
	}
	if len(ids) == 0 {
		return check{Name: "holding", Status: statusOK, Value: "없음"}
	}
	return check{Name: "holding", Status: statusOK, Value: strings.Join(ids, ", ")}
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
