package board

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ExportDecisions 는 결정마다 ADR 마크다운 파일 하나를 만든다 (§3.9, features §5.5).
//
// ref 가 진실이고 파일은 파생물이다. 대상 디렉터리에 같은 이름이 있으면
// 덮어쓴다 — 파생물이므로 손으로 고친 내용을 지키는 것이 오히려 해롭다.
// 다만 이 파일들은 export 한 뒤 평범하게 커밋한다. 사람이 코드 리뷰와 PR 에서
// 읽을 수 있어야 결정 기록의 값이 나오기 때문이다.
func (b *Board) ExportDecisions(dir string) ([]string, error) {
	entries, err := b.ListDecisions(DecisionListOptions{All: true})
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("출력 디렉터리 생성: %w", err)
	}

	written := make([]string, 0, len(entries))
	for _, entry := range entries {
		decision, err := b.ShowDecision(entry.ID)
		if err != nil {
			continue
		}
		path := filepath.Join(dir, decision.ID+".md")
		if err := os.WriteFile(path, decision.Markdown(entry.SupersededBy), 0o644); err != nil {
			return written, err
		}
		written = append(written, path)
	}
	return written, nil
}

// Markdown 은 ADR 한 편이다.
//
// supersededBy 를 인자로 받는 이유는 그것이 저장된 값이 아니기 때문이다.
// 결정 문서 하나만 봐서는 알 수 없고, 목록 전체에서 계산된다 (§3.9).
func (d *Decision) Markdown(supersededBy []string) []byte {
	var out strings.Builder
	fmt.Fprintf(&out, "<!-- generated: %s -->\n", d.DecidedAt)
	fmt.Fprintf(&out, "<!-- %s -->\n\n", DerivedWarning)
	fmt.Fprintf(&out, "# %s  %s\n\n", d.ID, d.Title)

	fmt.Fprintf(&out, "- 결정 시각: %s\n", d.DecidedAt)
	fmt.Fprintf(&out, "- 결정자: %s\n", d.DecidedBy)
	if d.Plan != "" {
		fmt.Fprintf(&out, "- Plan: %s\n", d.Plan)
	}
	if len(d.Issues) > 0 {
		fmt.Fprintf(&out, "- Issues: %s\n", strings.Join(d.Issues, ", "))
	}
	if d.Supersedes != "" {
		fmt.Fprintf(&out, "- Supersedes: %s\n", d.Supersedes)
	}
	if len(supersededBy) > 0 {
		fmt.Fprintf(&out, "- Superseded by: %s\n", strings.Join(supersededBy, ", "))
	}

	for _, section := range []struct{ title, body string }{
		{"Context", d.Context},
		{"Decision", d.Chosen},
		{"Consequences", d.Consequences},
	} {
		if section.body == "" {
			continue
		}
		fmt.Fprintf(&out, "\n## %s\n\n%s\n", section.title, section.body)
	}
	if len(d.Options) > 0 {
		fmt.Fprintf(&out, "\n## Options\n\n")
		for _, option := range d.Options {
			marker := "-"
			if option == d.Chosen {
				// 택한 것을 표시한다. 목록만 보고 결론을 되짚을 수 있어야 한다.
				marker = "- **(택함)**"
			}
			fmt.Fprintf(&out, "%s %s\n", marker, option)
		}
	}
	if len(d.Body) > 0 {
		fmt.Fprintf(&out, "\n## 근거\n\n%s", d.Body)
	}
	return []byte(out.String())
}
