package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// watch 는 첫 주기에 베이스라인만 잡고, 이후 변경을 줄당 JSON 으로 낸다.
//
// SIGINT 대신 context 취소로 끝낸다 — 같은 경로를 탄다.
func TestWatchStreamsJSONLines(t *testing.T) {
	dir := doctorRepo(t)
	runCLI(t, dir, "add", "이미 있음")

	ctx, cancel := context.WithCancel(context.Background())
	var stdout bytes.Buffer
	root := New(Version{CLI: "test", Schema: "1"}, &stdout, io.Discard)

	done := make(chan error, 1)
	go func() {
		done <- root.Run(ctx, []string{"ppwk", "-C", dir, "watch", "--interval", "20ms"})
	}()

	// 베이스라인이 잡힐 시간을 준 뒤 변경한다.
	time.Sleep(150 * time.Millisecond)
	runCLI(t, dir, "add", "새 이슈")
	time.Sleep(200 * time.Millisecond)
	cancel()

	if err := <-done; err != nil {
		t.Fatalf("watch = %v", err)
	}

	var events []map[string]any
	for line := range strings.Lines(stdout.String()) {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("줄이 JSON 이 아닙니다: %q", line)
		}
		events = append(events, event)
	}
	if len(events) != 1 {
		t.Fatalf("이벤트 %d개: %v", len(events), events)
	}
	if events[0]["kind"] != "created" || events[0]["id"] != "T002" {
		t.Fatalf("event = %v", events[0])
	}
}
