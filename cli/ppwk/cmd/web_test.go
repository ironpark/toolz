package cmd

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/web"
)

// web 은 초기화되지 않은 저장소에서 무엇을 해야 하는지 알려주고 멈춘다.
//
// 서버를 띄워 놓고 빈 화면을 보여주면 사용자는 무엇이 잘못됐는지 모른다.
func TestWebRequiresInitializedBoard(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.com"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v\n%s", err, out)
		}
	}

	out, err := runCLIErr(t, dir, "web", "--no-open", "--addr", "127.0.0.1:0")
	if code := exitCode(t, err); code != ExitUsage {
		t.Fatalf("exit = %d, want %d\n%s\n%v", code, ExitUsage, out, err)
	}
	if !strings.Contains(err.Error(), "ppwk init") {
		t.Fatalf("무엇을 해야 하는지 알려주지 않습니다: %v", err)
	}
}

// 웹 화면은 별도 배포물이 아니다. 바이너리 하나에 들어 있어야 한다.
func TestWebAssetsAreEmbedded(t *testing.T) {
	file, err := web.Assets().Open("index.html")
	if err != nil {
		t.Fatalf("index.html 이 바이너리에 없습니다: %v", err)
	}
	file.Close()
}
