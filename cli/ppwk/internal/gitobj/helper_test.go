package gitobj

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func writeFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0o644)
}

// runGitStdin 은 stdin 을 주며 git 을 실행한다.
func runGitStdin(t *testing.T, dir, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out))
}
