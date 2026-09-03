// Package agentdocs 는 init 이 만드는 에이전트 문서다 (features §1.1).
//
// 분리 이유는 context 예산이다. 에이전트는 매 세션 AGENTS.md 를 싣기 때문에
// 여기에 전체 매뉴얼을 담으면 실제 작업에 쓸 토큰이 줄어든다.
package agentdocs

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
)

//go:embed files
var files embed.FS

// DocsDir 은 하위 문서가 놓이는 경로다.
const DocsDir = "docs/ppwk"

// EntryPoint 는 항상 로드되는 진입점 파일이다.
const EntryPoint = "AGENTS.md"

// MaxEntryPointLines 는 AGENTS.md 의 줄 수 예산이다.
//
// 매 세션 로드되므로 증가를 테스트로 막는다 (T1.16).
const MaxEntryPointLines = 80

// Files 는 저장소 상대 경로 → 내용이다.
func Files() (map[string][]byte, error) {
	entries, err := files.ReadDir("files")
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		// embed.FS 는 항상 슬래시 경로다. filepath 를 쓰면 Windows 에서 어긋난다.
		data, err := files.ReadFile(path.Join("files", entry.Name()))
		if err != nil {
			return nil, err
		}
		out[destPath(entry.Name())] = data
	}
	return out, nil
}

// destPath 는 템플릿 이름을 저장소 안의 경로로 옮긴다.
func destPath(name string) string {
	if name == EntryPoint+".tmpl" {
		return EntryPoint
	}
	return filepath.Join(DocsDir, name)
}

// Write 는 문서를 저장소에 만든다.
//
// 이미 있는 파일은 건드리지 않는다. 판단은 파일 단위이므로 일부만 있으면
// 없는 것만 생긴다 (T1.13). 만든 파일 목록을 정렬해 돌려준다.
func Write(root string) (created []string, err error) {
	docs, err := Files()
	if err != nil {
		return nil, err
	}
	for rel, data := range docs {
		abs := filepath.Join(root, rel)
		if _, err := os.Stat(abs); err == nil {
			continue // 이미 있다. 사용자가 고쳤을 수 있으므로 덮지 않는다.
		} else if !os.IsNotExist(err) {
			return created, fmt.Errorf("%s: %w", rel, err)
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return created, err
		}
		if err := os.WriteFile(abs, data, 0o644); err != nil {
			return created, fmt.Errorf("%s: %w", rel, err)
		}
		created = append(created, rel)
	}
	sort.Strings(created)
	return created, nil
}
