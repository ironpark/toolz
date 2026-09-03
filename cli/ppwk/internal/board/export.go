package board

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// DerivedWarning 은 생성물 헤더에 넣는 경고다 (§5.4).
//
// export 는 단방향이다. 이 문구가 없으면 누군가 반드시 생성 파일을 고치고
// 반영되기를 기다린다.
const DerivedWarning = "ppwk 가 생성한 단방향 파생물입니다. 이 파일을 편집해도 보드에 반영되지 않습니다."

// ExportOptions 는 내보내기 범위다.
type ExportOptions struct {
	// All 은 archive 도 포함한다.
	All bool
	// Plan 은 특정 plan 으로 제한한다.
	Plan string
}

// Export 는 내보낸 보드 한 벌이다.
//
// 스냅샷 일관성을 보장하지 않는다. 읽는 도중 다른 에이전트가 상태를 바꿀 수
// 있으며, 그것을 막으려면 보드 전체를 잠가야 한다 — 파생물 하나를 위해
// 치를 비용이 아니다.
type Export struct {
	Schema      int             `json:"schema"`
	GeneratedAt model.Timestamp `json:"generated_at"`
	Warning     string          `json:"warning"`
	Issues      []model.Issue   `json:"issues"`
}

// Export 는 이슈를 모아 내보낼 형태로 만든다.
func (b *Board) Export(opts ExportOptions) (*Export, error) {
	entries, err := b.List(ListOptions{All: opts.All, Plan: opts.Plan})
	if err != nil {
		return nil, err
	}
	out := &Export{
		Schema:      model.SchemaVersion,
		GeneratedAt: model.Now(),
		Warning:     DerivedWarning,
		Issues:      make([]model.Issue, 0, len(entries)),
	}
	for _, entry := range entries {
		issue, err := b.Show(entry.ID)
		if err != nil {
			// 손상된 이슈 하나가 내보내기 전체를 막지 않는다. fsck 가 잡는다.
			continue
		}
		out.Issues = append(out.Issues, issue.Issue)
	}
	return out, nil
}

// JSON 은 기계가 읽을 형태다. import 가 되돌려 넣을 수 있는 유일한 형식이다.
func (e *Export) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Markdown 은 사람이 읽을 형태다.
func (e *Export) Markdown() []byte {
	var out strings.Builder
	// 헤더를 주석으로 넣는다. 렌더링에는 보이지 않지만 파일을 여는 사람은
	// 맨 위에서 본다.
	fmt.Fprintf(&out, "<!-- generated: %s -->\n", e.GeneratedAt)
	fmt.Fprintf(&out, "<!-- %s -->\n\n", e.Warning)
	fmt.Fprintf(&out, "# 보드\n\n")
	fmt.Fprintf(&out, "생성: %s\n\n", e.GeneratedAt)
	fmt.Fprintf(&out, "> %s\n\n", e.Warning)

	fmt.Fprintf(&out, "| ID | 상태 | 우선순위 | 소유자 | plan | phase | 제목 |\n")
	fmt.Fprintf(&out, "|---|---|---|---|---|---|---|\n")
	for _, issue := range e.Issues {
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %s | %s | %s |\n",
			issue.ID, issue.Status, issue.Priority, dashOr(issue.Owner),
			dashOr(issue.Plan), dashOr(issue.Phase), escapePipes(issue.Title))
	}
	return []byte(out.String())
}

// CSV 는 표 도구로 넘길 형태다.
func (e *Export) CSV() ([]byte, error) {
	var out strings.Builder
	fmt.Fprintf(&out, "# generated: %s\n", e.GeneratedAt)
	fmt.Fprintf(&out, "# %s\n", e.Warning)

	w := csv.NewWriter(&out)
	rows := [][]string{{"id", "status", "priority", "owner", "plan", "phase", "seq", "title", "created_at", "updated_at"}}
	for _, issue := range e.Issues {
		rows = append(rows, []string{
			issue.ID, string(issue.Status), string(issue.Priority), issue.Owner,
			issue.Plan, issue.Phase, strconv.Itoa(issue.Seq), issue.Title,
			issue.CreatedAt.String(), issue.UpdatedAt.String(),
		})
	}
	if err := w.WriteAll(rows); err != nil {
		return nil, err
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return []byte(out.String()), nil
}

// Render 는 형식 이름으로 골라 낸다.
func (e *Export) Render(format string) ([]byte, error) {
	switch format {
	case "json", "":
		return e.JSON()
	case "md", "markdown":
		return e.Markdown(), nil
	case "csv":
		return e.CSV()
	}
	return nil, fmt.Errorf("알 수 없는 형식입니다: %q (json|md|csv)", format)
}

func dashOr(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// escapePipes 는 제목의 | 가 표를 깨지 않게 한다.
func escapePipes(s string) string {
	return strings.ReplaceAll(s, "|", "\\|")
}
