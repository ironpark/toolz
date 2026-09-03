package session

import (
	"os"
	"strings"
	"testing"
)

// nonce 는 128비트이고 실행마다 달라야 한다 (§4.3).
func TestNewNonceIsUnique(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		n := NewNonce()
		if len(n) != nonceBytes*2 {
			t.Fatalf("nonce 길이 %d, want %d", len(n), nonceBytes*2)
		}
		if seen[n] {
			t.Fatalf("nonce 가 중복됐습니다: %s", n)
		}
		seen[n] = true
	}
}

// 세션을 감지할 수 없어도 비워 두지 않는다. 비면 OID 가 겹친다 (§4.3).
func TestResolveAlwaysHasSession(t *testing.T) {
	t.Setenv("PPWK_SESSION", "")
	os.Unsetenv("PPWK_SESSION")

	first := Resolve(Options{Flag: "agent-a", Worktree: t.TempDir()})
	if first.Session == "" {
		t.Fatal("Session 이 비어 있습니다")
	}
	second := Resolve(Options{Flag: "agent-a", Worktree: t.TempDir()})
	if first.SessionSource == "generated nonce" && first.Session == second.Session {
		t.Fatal("생성한 nonce 가 두 실행에서 같습니다")
	}
}

// PPWK_SESSION 이 있으면 그것이 이긴다.
func TestResolvePrefersEnvSession(t *testing.T) {
	t.Setenv("PPWK_SESSION", "from-env")
	got := Resolve(Options{Flag: "agent-a", Worktree: t.TempDir()})
	if got.Session != "from-env" || got.SessionSource != "PPWK_SESSION" {
		t.Fatalf("Session = %q (%s)", got.Session, got.SessionSource)
	}
}

// 같은 셸에서 실행한 명령들은 같은 세션으로 묶여야 한다.
//
// 이것이 깨지면 훅 없는 사용자의 claim 다음 start 가 자기 것이 아니게 되고,
// list --mine 은 언제나 비어 있게 된다.
func TestShellSessionIsStableAcrossCalls(t *testing.T) {
	first, ok := ShellSession()
	if !ok {
		t.Skip("부모 프로세스를 알 수 없는 환경입니다")
	}
	second, _ := ShellSession()
	if first != second {
		t.Fatalf("같은 프로세스에서 %q != %q", first, second)
	}
	if !strings.HasPrefix(first, "shell-") {
		t.Fatalf("세션 = %q", first)
	}
	// nonce 와 달라야 한다 — 매번 새로 만드는 값이면 안 된다.
	if first == NewNonce() {
		t.Fatal("nonce 와 같습니다")
	}
}
