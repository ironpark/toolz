package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
	"github.com/ironpark/toolz/cli/ppwk/internal/toolhook"
)

// sessionEvent 는 도구가 stdin 으로 주는 훅 페이로드다.
//
// 아는 필드만 읽는다. 훅 이벤트의 JSON 스키마는 문서화된 안정 API 가 아니라
// 도구 버전에 따라 바뀔 수 있다 (§3.8).
type sessionEvent struct {
	SessionID string `json:"session_id"`
	CWD       string `json:"cwd"`
	Event     string `json:"hook_event_name"`
}

// runSessionEvent 는 도구 훅에서만 불린다 (features §4).
//
// **무슨 일이 있어도 exit 0 이다.** 훅은 도구 세션 안에서 동기 실행되므로,
// 여기서 실패를 올리면 사용자의 세션 시작이 막히거나 종료가 지연된다. 정합성은
// 층 1(잠금)이 단독으로 보장하므로 이 층은 실패해도 안전하다 (§3.8).
func runSessionEvent(x *ctx) error {
	event, ok := decodeSessionEvent(x)
	if !ok {
		return nil
	}
	b, err := hookBoard(x, event)
	if err != nil {
		return nil
	}

	switch event.Event {
	case "SessionStart":
		// 훅의 부모는 구조적으로 도구 프로세스다. 트리를 뒤질 필요가 없다 (D11).
		_ = b.RegisterHookSession(os.Getppid())
	case "SessionEnd":
		// claimed 만 반납한다. working 에는 worktree 의 미커밋 변경이 있을 수
		// 있고, 사용자가 도구를 닫았다 다시 열어 잇는 것은 흔하다 (D15).
		// working 은 층 1 의 생존 판정에 맡긴다.
		_, _ = b.ReleaseMine(board.TransitionOptions{Message: "SessionEnd"})
	}
	return nil
}

// decodeSessionEvent 는 stdin 을 읽는다. 읽을 수 없으면 조용히 포기한다.
func decodeSessionEvent(x *ctx) (sessionEvent, bool) {
	data, err := readAll(x.cmd.Reader)
	if err != nil || len(data) == 0 {
		return sessionEvent{}, false
	}
	var event sessionEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return sessionEvent{}, false
	}
	if event.Event != "SessionStart" && event.Event != "SessionEnd" {
		// 모르는 이벤트다. SubagentStart 처럼 우리가 등록하지 않은 것이
		// 흘러 들어올 수 있다.
		return sessionEvent{}, false
	}
	return event, true
}

// hookBoard 는 페이로드의 cwd 를 기준으로 보드를 연다.
//
// 훅은 도구가 spawn 하므로 실행 디렉터리가 저장소 밖일 수 있다. cwd 가 우리와
// 무관한 곳이면 여기서 실패하고, 호출자가 조용히 끝낸다.
func hookBoard(x *ctx, event sessionEvent) (*board.Board, error) {
	path := event.CWD
	if path == "" {
		path = x.cmd.String("C")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	return board.OpenFor(path, board.OpenOptions{
		Session: session.Options{
			Flag: x.cmd.String("agent"), Worktree: abs,
			// session_id 가 없으면 감지에 맡긴다. 훅은 도구의 자식이라
			// 환경변수가 닿지 않을 수 있으므로 이 값이 가장 정확하다.
			Session: event.SessionID,
		},
		// 훅은 사람이 부른 명령이 아니다. worktree 배타로 세션 시작을
		// 막으면 안 된다 — 그 판단은 실제 상태 변경 명령이 한다.
		AllowSharedWorktree: true,
	})
}

// runHookInstall 은 도구 훅을 설치한다 (§6).
func runHookInstall(x *ctx) error {
	tools, err := selectedTools(x)
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	force := x.cmd.Bool("force")
	for _, tool := range tools {
		if err := tool.Install(b.Root(), force); err != nil {
			return err
		}
		x.printf("installed  %s  %s\n", tool.Name, tool.Path)
	}
	if _, ok := toolByFlag(x, "codex"); ok {
		// 실험적 기능이라 기본 비활성이고, 프로젝트 로컬 훅은 신뢰 검토를
		// 거쳐야 실행된다. 설치만으로 동작하지 않을 수 있음을 알린다.
		fmt.Fprintln(x.stderr,
			"note: Codex 훅은 실험적 기능입니다. /hooks 에서 신뢰 검토를 거쳐야 실행되며 Windows 는 지원하지 않습니다.")
	}
	return nil
}

// runHookUninstall 은 ppwk 가 등록한 항목만 걷어낸다.
func runHookUninstall(x *ctx) error {
	tools, err := selectedTools(x)
	if err != nil {
		return err
	}
	b, err := x.board()
	if err != nil {
		return err
	}
	for _, tool := range tools {
		if err := tool.Uninstall(b.Root()); err != nil {
			return err
		}
		x.printf("removed  %s  %s\n", tool.Name, tool.Path)
	}
	return nil
}

// runHookStatus 는 도구별 이벤트 등록 상태를 낸다.
func runHookStatus(x *ctx) error {
	b, err := x.board()
	if err != nil {
		return err
	}
	statuses := make([]toolhook.Status, 0, len(toolhook.Tools))
	for _, tool := range toolhook.Tools {
		statuses = append(statuses, tool.Status(b.Root()))
	}
	if x.json {
		return x.emit(statuses)
	}
	rows := make([][]string, 0, len(statuses))
	for _, status := range statuses {
		if !status.Configured {
			rows = append(rows, []string{status.Tool, "not configured", status.Path})
			continue
		}
		marks := ""
		for _, event := range toolhook.Events {
			mark := "✗"
			if status.Events[event] {
				mark = "✓"
			}
			marks += fmt.Sprintf("%s %s  ", event, mark)
		}
		rows = append(rows, []string{status.Tool, marks, status.Path})
	}
	return x.table(rows)
}

// selectedTools 는 플래그로 고른 도구다. 아무것도 고르지 않으면 사용법 오류다.
func selectedTools(x *ctx) ([]toolhook.Tool, error) {
	if x.cmd.Bool("agent-tools") {
		return toolhook.Tools, nil
	}
	var tools []toolhook.Tool
	for _, tool := range toolhook.Tools {
		if x.cmd.Bool(tool.Name) {
			tools = append(tools, tool)
		}
	}
	if len(tools) == 0 {
		return nil, UsageError("대상 도구가 필요합니다 (--claude-code, --codex, --agent-tools)")
	}
	return tools, nil
}

func toolByFlag(x *ctx, name string) (toolhook.Tool, bool) {
	if !x.cmd.Bool(name) && !x.cmd.Bool("agent-tools") {
		return toolhook.Tool{}, false
	}
	return toolhook.ToolByName(name)
}
