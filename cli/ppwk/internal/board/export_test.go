package board

import (
	"encoding/csv"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ironpark/toolz/cli/ppwk/internal/model"
)

// T8.1 export json 이 유효한 JSON 이고 이슈를 담는다.
func TestExportJSONIsValid(t *testing.T) {
	b := initBoard(t)
	issue := mustAdd(t, b, AddOptions{Title: "대상", Priority: model.PriorityHigh,
		Labels: []string{"go"}, Body: []byte("본문\n")})

	data, err := b.Export(ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := data.Render("json")
	if err != nil {
		t.Fatal(err)
	}

	var decoded Export
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("유효한 JSON 이 아닙니다: %v\n%s", err, raw)
	}
	if decoded.Warning != DerivedWarning {
		t.Fatalf("warning = %q", decoded.Warning)
	}
	if decoded.GeneratedAt.Time.IsZero() {
		t.Fatal("generated_at 이 비어 있습니다")
	}
	if len(decoded.Issues) != 1 || decoded.Issues[0].ID != issue.ID {
		t.Fatalf("issues = %v", decoded.Issues)
	}
	if decoded.Issues[0].Priority != model.PriorityHigh {
		t.Fatalf("priority = %s", decoded.Issues[0].Priority)
	}
}

// T8.2 md 헤더에 생성 시각과 파생물 경고가 들어간다.
func TestExportMarkdownHeader(t *testing.T) {
	b := initBoard(t)
	mustAdd(t, b, AddOptions{Title: "제목에 | 가 있음"})

	data, err := b.Export(ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := data.Render("md")
	if err != nil {
		t.Fatal(err)
	}
	out := string(rendered)

	if !strings.Contains(out, data.GeneratedAt.String()) {
		t.Fatalf("생성 시각이 없습니다:\n%s", out)
	}
	if !strings.Contains(out, DerivedWarning) {
		t.Fatalf("파생물 경고가 없습니다:\n%s", out)
	}
	// 헤더는 맨 위여야 한다. 아래쪽에 있으면 파일을 여는 사람이 못 본다.
	header, _, _ := strings.Cut(out, "\n")
	if !strings.Contains(header, "generated") {
		t.Fatalf("첫 줄 = %q", header)
	}
	// 제목의 | 가 표를 깨지 않는다.
	if !strings.Contains(out, `제목에 \| 가 있음`) {
		t.Fatalf("| 가 이스케이프되지 않았습니다:\n%s", out)
	}
}

// csv 도 헤더에 경고를 담고, 나머지는 표준 CSV 로 읽힌다.
func TestExportCSV(t *testing.T) {
	b := initBoard(t)
	mustAdd(t, b, AddOptions{Title: "쉼표, 가 있는 제목"})

	data, err := b.Export(ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := data.Render("csv")
	if err != nil {
		t.Fatal(err)
	}
	out := string(rendered)
	if !strings.Contains(out, DerivedWarning) {
		t.Fatalf("파생물 경고가 없습니다:\n%s", out)
	}

	body := out[strings.Index(out, "id,"):]
	rows, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatalf("유효한 CSV 가 아닙니다: %v\n%s", err, body)
	}
	if len(rows) != 2 || rows[1][7] != "쉼표, 가 있는 제목" {
		t.Fatalf("rows = %v", rows)
	}
}

// --all 은 archive 를 포함하고, 기본은 제외한다.
func TestExportScopes(t *testing.T) {
	b := initBoard(t)
	live := mustAdd(t, b, AddOptions{Title: "살아있음"})
	gone := mustAdd(t, b, AddOptions{Title: "끝남"})
	transitionAll(t, b, gone.ID, ActionStart, ActionDone)

	for _, tc := range []struct {
		name string
		opts ExportOptions
		want []string
	}{
		{"기본", ExportOptions{}, []string{live.ID}},
		{"all", ExportOptions{All: true}, []string{live.ID, gone.ID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := b.Export(tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			if len(data.Issues) != len(tc.want) {
				t.Fatalf("%d개: %v", len(data.Issues), data.Issues)
			}
			for i, id := range tc.want {
				if data.Issues[i].ID != id {
					t.Fatalf("issues[%d] = %s, want %s", i, data.Issues[i].ID, id)
				}
			}
		})
	}
}

// 알 수 없는 형식은 조용히 json 으로 넘어가지 않고 거부한다.
func TestExportRejectsUnknownFormat(t *testing.T) {
	e := &Export{}
	if _, err := e.Render("yaml"); err == nil {
		t.Fatal("알 수 없는 형식을 받아들였습니다")
	}
}
