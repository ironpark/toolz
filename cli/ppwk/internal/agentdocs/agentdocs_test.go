package agentdocs

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

// T1.12 init 이 만들 문서가 전부 준비돼 있다.
func TestFilesCoversSpec(t *testing.T) {
	docs, err := Files()
	if err != nil {
		t.Fatalf("Files() = %v", err)
	}
	want := []string{
		"AGENTS.md",
		"docs/ppwk/authoring.md",
		"docs/ppwk/decisions.md",
		"docs/ppwk/git-behavior.md",
		"docs/ppwk/plans.md",
		"docs/ppwk/project.md",
		"docs/ppwk/query.md",
		"docs/ppwk/states.md",
		"docs/ppwk/troubleshooting.md",
	}
	for _, path := range want {
		if _, ok := docs[path]; !ok {
			t.Fatalf("%s 가 없습니다. 있는 것: %v", path, keys(docs))
		}
	}
	if len(docs) != len(want) {
		t.Fatalf("문서 %d개, want %d개: %v", len(docs), len(want), keys(docs))
	}
}

// T1.15 AGENTS.md 의 모든 상대 링크가 실제 파일을 가리킨다.
//
// 파일명을 바꾸면 이 테스트가 실패해야 한다.
func TestEntryPointLinksResolve(t *testing.T) {
	docs, err := Files()
	if err != nil {
		t.Fatalf("Files() = %v", err)
	}
	entry, ok := docs[EntryPoint]
	if !ok {
		t.Fatalf("%s 가 없습니다", EntryPoint)
	}

	links := regexp.MustCompile(`\]\(([^)]+)\)`).FindAllSubmatch(entry, -1)
	if len(links) == 0 {
		t.Fatal("AGENTS.md 에 링크가 하나도 없습니다")
	}
	for _, match := range links {
		target := string(match[1])
		if _, ok := docs[target]; !ok {
			t.Fatalf("깨진 링크: %s (있는 것: %v)", target, keys(docs))
		}
	}
}

// T1.16 AGENTS.md 가 크기 예산 이내다.
//
// 매 세션 컨텍스트에 실리므로 증가를 테스트로 막는다.
func TestEntryPointWithinBudget(t *testing.T) {
	docs, err := Files()
	if err != nil {
		t.Fatalf("Files() = %v", err)
	}
	lines := bytes.Count(docs[EntryPoint], []byte("\n"))
	if lines > MaxEntryPointLines {
		t.Fatalf("%s 가 %d줄입니다. 예산 %d줄 — 내용을 docs/ppwk/ 로 옮기세요",
			EntryPoint, lines, MaxEntryPointLines)
	}
}

// 하위 문서는 자족적이어야 하므로 진입점으로 돌아가는 링크를 갖는다.
func TestSubDocsLinkBack(t *testing.T) {
	docs, err := Files()
	if err != nil {
		t.Fatalf("Files() = %v", err)
	}
	for path, data := range docs {
		if path == EntryPoint {
			continue
		}
		if !bytes.Contains(data, []byte("AGENTS.md")) {
			t.Fatalf("%s 에 AGENTS.md 로 돌아가는 링크가 없습니다", path)
		}
	}
}

func TestWriteCreatesAll(t *testing.T) {
	root := t.TempDir()

	created, err := Write(root)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if len(created) != 9 {
		t.Fatalf("생성 %d개, want 9개: %v", len(created), created)
	}
	for _, rel := range created {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
	}
}

// T1.13 기존 파일은 덮어쓰지 않는다. 판단은 파일 단위다.
func TestWriteSkipsExisting(t *testing.T) {
	root := t.TempDir()

	custom := []byte("내가 고친 내용\n")
	if err := os.WriteFile(filepath.Join(root, EntryPoint), custom, 0o644); err != nil {
		t.Fatalf("WriteFile() = %v", err)
	}

	created, err := Write(root)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if slices.Contains(created, EntryPoint) {
		t.Fatalf("%s 를 덮어썼습니다: %v", EntryPoint, created)
	}
	// 없던 것은 생겼어야 한다.
	if len(created) != 8 {
		t.Fatalf("생성 %d개, want 8개: %v", len(created), created)
	}

	got, err := os.ReadFile(filepath.Join(root, EntryPoint))
	if err != nil {
		t.Fatalf("ReadFile() = %v", err)
	}
	if !bytes.Equal(got, custom) {
		t.Fatalf("%s 의 내용이 바뀌었습니다", EntryPoint)
	}
}

// 두 번 실행해도 안전하다.
func TestWriteIdempotent(t *testing.T) {
	root := t.TempDir()

	if _, err := Write(root); err != nil {
		t.Fatalf("Write() = %v", err)
	}
	created, err := Write(root)
	if err != nil {
		t.Fatalf("Write() = %v", err)
	}
	if len(created) != 0 {
		t.Fatalf("두 번째 실행이 %v 를 만들었습니다", created)
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
