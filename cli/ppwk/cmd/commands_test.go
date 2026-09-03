package cmd

import (
	"io"
	"slices"
	"testing"

	"github.com/urfave/cli/v3"
)

// T3.8 배정 관련 명령이 존재하지 않는다 (§8.0).
//
// 배정은 오케스트레이터의 일이지 보드의 일이 아니다. 이 명령들이 다시 생기면
// 큐가 두 개가 되고, 어느 쪽이 진실인지 알 수 없게 된다. 재도입 방지 회귀다.
func TestAssignmentCommandsAbsent(t *testing.T) {
	banned := []string{"assign", "unassign", "inbox", "accept", "reject"}
	root := New(Version{CLI: "test", Schema: "1"}, io.Discard, io.Discard)

	var walk func(cmds []*cli.Command, path string)
	walk = func(cmds []*cli.Command, path string) {
		for _, c := range cmds {
			full := path + c.Name
			if slices.Contains(banned, c.Name) {
				t.Fatalf("%q 명령이 존재합니다 — 배정은 오케스트레이터 담당입니다 (§8.0)", full)
			}
			for _, alias := range c.Aliases {
				if slices.Contains(banned, alias) {
					t.Fatalf("%q 의 별칭 %q 가 배정 명령입니다 (§8.0)", full, alias)
				}
			}
			walk(c.Commands, full+" ")
		}
	}
	walk(root.Commands, "")
}

// 전이 명령이 전부 배선돼 있어야 한다. notImplemented 로 남으면 안 된다.
func TestTransitionCommandsWired(t *testing.T) {
	root := New(Version{CLI: "test", Schema: "1"}, io.Discard, io.Discard)
	want := []string{"claim", "start", "done", "block", "unblock", "release", "cancel", "history"}

	for _, name := range want {
		var found *cli.Command
		for _, c := range root.Commands {
			if c.Name == name {
				found = c
				break
			}
		}
		if found == nil {
			t.Fatalf("%q 명령이 없습니다", name)
		}
		if found.Action == nil {
			t.Fatalf("%q 에 Action 이 없습니다", name)
		}
	}
}
