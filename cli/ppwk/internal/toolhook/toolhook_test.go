package toolhook

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func read(t *testing.T, root string, tool Tool) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, tool.Path))
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v\n%s", tool.Path, err, data)
	}
	return out
}

// T11.9 두 도구에 같은 명령이 등록된다.
func TestInstallsSameCommandToBothTools(t *testing.T) {
	root := t.TempDir()
	for _, tool := range Tools {
		if err := tool.Install(root, false); err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		status := tool.Status(root)
		if !status.Installed() {
			t.Fatalf("%s = %+v", tool.Name, status)
		}
		data, err := os.ReadFile(filepath.Join(root, tool.Path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), Command) {
			t.Fatalf("%s 에 명령이 없습니다:\n%s", tool.Name, data)
		}
	}
}

// T11.11 서브에이전트 이벤트에는 등록하지 않는다.
//
// 서브에이전트는 별도 세션이 아니라 부모 세션의 일부다. 걸면 서브에이전트마다
// 유령 에이전트가 생긴다 (§3.8).
func TestSubagentEventsNotRegistered(t *testing.T) {
	root := t.TempDir()
	for _, tool := range Tools {
		if err := tool.Install(root, false); err != nil {
			t.Fatal(err)
		}
		hooks, _ := read(t, root, tool)["hooks"].(map[string]any)
		for name := range hooks {
			if strings.HasPrefix(name, "Subagent") {
				t.Fatalf("%s 에 %q 가 등록됐습니다", tool.Name, name)
			}
		}
		if len(hooks) != len(Events) {
			t.Fatalf("%s 이벤트 = %v, want %v", tool.Name, hooks, Events)
		}
	}
}

// T11.10 기존 설정은 병합하고, 우리 것으로 보이는데 다르면 중단한다.
func TestInstallMergesAndDetectsConflict(t *testing.T) {
	tool := Tools[0] // claude-code
	root := t.TempDir()
	path := filepath.Join(root, tool.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{
	  "model": "opus",
	  "hooks": {
	    "SessionStart": [
	      {"hooks": [{"type": "command", "command": "echo 남의 훅"}]}
	    ]
	  }
	}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := tool.Install(root, false); err != nil {
		t.Fatal(err)
	}
	config := read(t, root, tool)
	// 우리가 모르는 설정은 보존된다. 사람이 쓰는 파일이다.
	if config["model"] != "opus" {
		t.Fatalf("모르는 키가 사라졌습니다: %v", config)
	}
	// 남의 훅과 나란히 선다. 병합이지 덮어쓰기가 아니다.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "남의 훅") || !strings.Contains(string(data), Command) {
		t.Fatalf("병합되지 않았습니다:\n%s", data)
	}

	// 두 번 설치해도 늘어나지 않는다. 멱등하다.
	if err := tool.Install(root, false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(mustRead(t, path)), Command); got != len(Events) {
		t.Fatalf("명령이 %d번 등록됐습니다, want %d", got, len(Events))
	}

	// 옛 경로의 ppwk 훅이 남아 있으면 충돌이다.
	stale := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/old/ppwk internal session-event"}]}]}}`
	if err := os.WriteFile(path, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	err = tool.Install(root, false)
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Install() = %v, want ErrConflict", err)
	}
	// --force 면 우리 것만 갈아끼운다.
	if err := tool.Install(root, true); err != nil {
		t.Fatal(err)
	}
	data = mustRead(t, path)
	if strings.Contains(string(data), "/old/ppwk") {
		t.Fatalf("옛 항목이 남았습니다:\n%s", data)
	}
	if !strings.Contains(string(data), Command) {
		t.Fatalf("새 항목이 없습니다:\n%s", data)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// uninstall 은 우리 항목만 걷어낸다.
func TestUninstallLeavesForeignHooks(t *testing.T) {
	tool := Tools[0]
	root := t.TempDir()
	path := filepath.Join(root, tool.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"echo 남의 훅"}]}]}}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tool.Install(root, false); err != nil {
		t.Fatal(err)
	}
	if err := tool.Uninstall(root); err != nil {
		t.Fatal(err)
	}

	data := mustRead(t, path)
	if strings.Contains(string(data), Command) {
		t.Fatalf("우리 항목이 남았습니다:\n%s", data)
	}
	if !strings.Contains(string(data), "남의 훅") {
		t.Fatalf("남의 훅을 지웠습니다:\n%s", data)
	}
	if tool.Status(root).Installed() {
		t.Fatal("status 가 여전히 설치됨입니다")
	}

	// 설정이 없어도 uninstall 은 성공한다. 멱등하다.
	if err := tool.Uninstall(t.TempDir()); err != nil {
		t.Fatalf("설정이 없을 때 = %v", err)
	}
}

// T11.12 status 가 이벤트별 등록 여부를 구분해 보여준다.
func TestStatusReportsPerEvent(t *testing.T) {
	tool := Tools[0]
	root := t.TempDir()

	status := tool.Status(root)
	if status.Configured || status.Installed() {
		t.Fatalf("설정 없음 = %+v", status)
	}

	path := filepath.Join(root, tool.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	half := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"` + Command + `"}]}]}}`
	if err := os.WriteFile(path, []byte(half), 0o644); err != nil {
		t.Fatal(err)
	}
	status = tool.Status(root)
	if !status.Configured || status.Installed() {
		t.Fatalf("절반만 설치 = %+v", status)
	}
	if !status.Events["SessionStart"] || status.Events["SessionEnd"] {
		t.Fatalf("이벤트별 = %+v", status.Events)
	}
}

// 깨진 설정 파일은 조용히 덮어쓰지 않고 오류로 알린다.
func TestInstallRefusesBrokenConfig(t *testing.T) {
	tool := Tools[0]
	root := t.TempDir()
	path := filepath.Join(root, tool.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{이건 JSON 이 아니다"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tool.Install(root, false); err == nil {
		t.Fatal("깨진 설정을 덮어썼습니다")
	}
}
