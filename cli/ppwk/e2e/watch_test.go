package e2e

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// watcher 는 watch 를 띄우고 이벤트를 모은다.
type watcher struct {
	t      *testing.T
	cmd    *exec.Cmd
	mu     sync.Mutex
	events []map[string]any
}

// watch 는 polling watcher 를 시작한다.
func (f *Fixture) watch(args ...string) *watcher {
	f.t.Helper()
	w := &watcher{t: f.t}
	w.cmd = exec.Command(binary, append([]string{"--json", "watch", "--interval", "100ms"}, args...)...)
	w.cmd.Dir = f.Root
	w.cmd.Env = append(baseEnv(), f.Env...)
	stdout, err := w.cmd.StdoutPipe()
	if err != nil {
		f.t.Fatal(err)
	}
	w.cmd.Stderr = os.Stderr
	if err := w.cmd.Start(); err != nil {
		f.t.Fatal(err)
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			var event map[string]any
			if json.Unmarshal(scanner.Bytes(), &event) != nil {
				continue
			}
			w.mu.Lock()
			w.events = append(w.events, event)
			w.mu.Unlock()
		}
	}()
	f.t.Cleanup(w.Stop)
	return w
}

func (w *watcher) Stop() {
	if w.cmd.Process != nil {
		w.cmd.Process.Kill()
		w.cmd.Wait()
	}
}

// seen 은 주어진 ref 에 대한 이벤트가 왔는지다.
func (w *watcher) seen(ref, kind string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, event := range w.events {
		if event["ref"] == ref && (kind == "" || event["kind"] == kind) {
			return true
		}
	}
	return false
}

// E2E-18: git hook 경로는 없다.
//
// 채택하지 않은 이유는 §6.3 에 있다. 되살리고 싶어지는 종류라 회귀로 둔다.
func TestNoGitHookSurface(t *testing.T) {
	f := newFixture(t)
	var help strings.Builder
	help.WriteString(f.MustRun("--help").Stdout)
	for _, cmd := range []string{"init", "watch", "hook", "doctor", "next"} {
		help.WriteString(f.MustRun(cmd, "--help").Stdout)
	}
	for _, sub := range []string{"install", "uninstall", "status"} {
		help.WriteString(f.MustRun("hook", sub, "--help").Stdout)
	}
	for _, banned := range []string{"--git", "--hooks", "--hook ", "socket", "PPWK_SOCK", "reference-transaction"} {
		if strings.Contains(help.String(), banned) {
			t.Fatalf("도움말에 %q 가 있습니다", banned)
		}
	}

	// 실제로도 만들어지지 않는다.
	f.MustRun("hook", "install", "--agent-tools")
	path := filepath.Join(f.commonDir(), "hooks", "reference-transaction")
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s 가 만들어졌습니다", path)
	}
}

// E2E-20: polling 만으로 완결.
//
// 훅이 없으므로 일반 git 작업이 느려질 이유가 없다. 훅을 다시 넣으면 여기가
// 무너진다.
func TestPollingCoversFullWorkflow(t *testing.T) {
	f := newFixture(t)
	w := f.watch()

	id := f.add("작업")
	ref := "refs/ppwk/issues/" + id
	waitFor(t, 10*time.Second, "created 감지", func() bool { return w.seen(ref, "created") })

	f.MustRun("claim", id)
	waitFor(t, 10*time.Second, "updated 감지", func() bool { return w.seen(ref, "updated") })

	f.MustRun("start", id)
	f.MustRun("done", id)
	// done 은 이동이다. issues/ 에서 사라지고 archive/ 에 생긴다.
	waitFor(t, 10*time.Second, "deleted 감지", func() bool { return w.seen(ref, "deleted") })
	waitFor(t, 10*time.Second, "archive 감지", func() bool {
		return w.seen("refs/ppwk/archive/"+id, "created")
	})

	// 일반 git 작업에 훅이 끼어들지 않는다.
	for _, args := range [][]string{
		{"commit", "--quiet", "--allow-empty", "-m", "보통 커밋"},
		{"branch", "tmp-branch"},
		{"tag", "tmp-tag"},
	} {
		f.Git(args...)
	}
	if _, err := os.Stat(filepath.Join(f.commonDir(), "hooks", "reference-transaction")); err == nil {
		t.Fatal("git 훅이 설치돼 있습니다")
	}
}

// E2E-22: pack-refs 후 감지.
//
// 파일 mtime/inotify 기반 구현이면 실패한다 (§6.2). pack-refs 는 loose ref
// 파일을 통째로 없애므로, 파일을 보는 구현은 그 뒤의 변경을 놓친다.
func TestDetectionSurvivesPackRefs(t *testing.T) {
	f := newFixture(t)
	w := f.watch()
	// watch 는 시작 시점을 기준선으로 잡는다. 그 이후의 변경만 이벤트다.
	first := f.add("먼저")
	waitFor(t, 10*time.Second, "기준선", func() bool {
		return w.seen("refs/ppwk/issues/"+first, "created")
	})

	f.Git("pack-refs", "--all")
	if _, err := os.Stat(filepath.Join(f.commonDir(), "refs", "ppwk", "issues", first)); err == nil {
		t.Fatal("pack-refs 가 loose ref 를 남겼습니다 — 이 테스트의 전제가 깨졌습니다")
	}

	f.MustRun("claim", first)
	waitFor(t, 10*time.Second, "pack-refs 후 변경 감지", func() bool {
		return w.seen("refs/ppwk/issues/"+first, "updated")
	})
	second := f.add("나중")
	waitFor(t, 10*time.Second, "pack-refs 후 생성 감지", func() bool {
		return w.seen("refs/ppwk/issues/"+second, "created")
	})
}
