package web

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/session"
)

// newServer 는 초기화된 보드 위의 서버다.
func newServer(t *testing.T) (*Server, *board.Board) {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.name", "test"},
		{"config", "user.email", "test@example.invalid"},
		{"commit", "--quiet", "--allow-empty", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	b, err := board.Open(dir, session.Identity{Agent: "agent-a", Session: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Init(board.InitOptions{NoAgentsMD: true}); err != nil {
		t.Fatal(err)
	}
	return New(b, Options{PollInterval: 20 * time.Millisecond}), b
}

// get 은 요청을 보내고 응답을 돌려준다.
func do(t *testing.T, s *Server, method, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

// decodeData 는 봉투에서 data 를 꺼낸다.
func decodeData(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
		OK   bool            `json:"ok"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("봉투 파싱: %v\n%s", err, rec.Body)
	}
	if !envelope.OK {
		t.Fatalf("ok=false: %s", rec.Body)
	}
	if v != nil {
		if err := json.Unmarshal(envelope.Data, v); err != nil {
			t.Fatalf("data 파싱: %v\n%s", err, envelope.Data)
		}
	}
}

// 이슈를 만들고 전이시키는 한 바퀴가 API 로 돈다.
func TestIssueLifecycleOverAPI(t *testing.T) {
	s, _ := newServer(t)

	rec := do(t, s, "POST", "/api/issues", `{"title":"첫 작업","priority":"high"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("add = %d %s", rec.Code, rec.Body)
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	decodeData(t, rec, &created)
	if created.ID == "" || created.Status != "open" {
		t.Fatalf("created = %+v", created)
	}

	for _, step := range []struct {
		action string
		want   string
	}{{"claim", "claimed"}, {"start", "working"}, {"done", "done"}} {
		rec := do(t, s, "POST", "/api/issues/"+created.ID+"/actions/"+step.action, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d %s", step.action, rec.Code, rec.Body)
		}
		var issue struct {
			Status string `json:"status"`
		}
		decodeData(t, rec, &issue)
		if issue.Status != step.want {
			t.Fatalf("%s 후 status = %q, want %q", step.action, issue.Status, step.want)
		}
	}

	// done 은 archive 이동을 겸한다. 상세 조회는 여전히 찾아야 한다.
	rec = do(t, s, "GET", "/api/issues/"+created.ID, "")
	var detail struct {
		Archived bool `json:"archived"`
	}
	decodeData(t, rec, &detail)
	if !detail.Archived {
		t.Fatal("done 뒤에도 archived 가 아닙니다")
	}
}

// 도메인 오류가 HTTP 상태로 옮겨진다.
//
// 화면은 이 분류로 어조를 정한다. 전부 500 이면 "다시 하면 되는 일" 과
// "규칙 위반" 을 구분할 수 없다.
func TestErrorsMapToStatus(t *testing.T) {
	s, _ := newServer(t)
	do(t, s, "POST", "/api/issues", `{"title":"작업"}`)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
		status int
		kind   string
	}{
		{"없는 이슈", "GET", "/api/issues/T999", "", http.StatusNotFound, "not_found"},
		{"허용되지 않는 전이", "POST", "/api/issues/T001/actions/done", "", http.StatusConflict, "invalid_transition"},
		{"알 수 없는 전이", "POST", "/api/issues/T001/actions/explode", "", http.StatusBadRequest, "usage"},
		{"빈 제목", "POST", "/api/issues", `{"title":"  "}`, http.StatusBadRequest, "usage"},
		{"깨진 JSON", "POST", "/api/issues", `{`, http.StatusBadRequest, "usage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, s, tc.method, tc.path, tc.body)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d\n%s", rec.Code, tc.status, rec.Body)
			}
			var envelope struct {
				Error struct {
					Kind string `json:"kind"`
				} `json:"error"`
				OK bool `json:"ok"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
				t.Fatalf("파싱: %v\n%s", err, rec.Body)
			}
			if envelope.OK {
				t.Fatalf("ok=true 인데 오류여야 합니다: %s", rec.Body)
			}
			if envelope.Error.Kind != tc.kind {
				t.Fatalf("kind = %q, want %q", envelope.Error.Kind, tc.kind)
			}
		})
	}
}

// 알 수 없는 경로는 index.html 로 간다.
//
// 클라이언트 라우팅이라 /plans 같은 경로에 해당하는 파일이 없다. 404 를
// 돌려주면 새로고침이 깨진다.
func TestSPAFallback(t *testing.T) {
	s, _ := newServer(t)
	for _, path := range []string{"/", "/plans", "/decisions", "/agents", "/모르는/경로"} {
		rec := do(t, s, "GET", path, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("%s = %d", path, rec.Code)
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
			t.Fatalf("%s 의 Content-Type = %q", path, rec.Header().Get("Content-Type"))
		}
	}

	// API 는 fallback 대상이 아니다. 없는 API 는 없다고 말해야 한다.
	if rec := do(t, s, "GET", "/api/모르는것", ""); rec.Code == http.StatusOK {
		t.Fatalf("없는 API 가 200 입니다:\n%s", rec.Body)
	}
}

// 교차 출처 요청은 받지 않는다.
//
// 이 서버는 loopback 에 뜨지만 그것만으로는 부족하다. 사용자가 열어 둔 아무
// 웹 페이지의 스크립트도 127.0.0.1 로 요청을 보낼 수 있고, 이 API 는 보드를
// 바꾼다.
func TestCrossOriginIsRejected(t *testing.T) {
	s, _ := newServer(t)

	req := httptest.NewRequest("POST", "/api/issues", strings.NewReader(`{"title":"침입"}`))
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403\n%s", rec.Code, rec.Body)
	}

	// 우리 화면에서 온 것은 통과한다.
	req = httptest.NewRequest("POST", "/api/issues", strings.NewReader(`{"title":"정상"}`))
	req.Header.Set("Origin", "http://"+req.Host)
	rec = httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("같은 출처가 거부됐습니다: %d\n%s", rec.Code, rec.Body)
	}

	// 이슈가 실제로 만들어지지 않았어야 한다 — 거부는 응답만이 아니다.
	rec = do(t, s, "GET", "/api/issues", "")
	var entries []struct {
		Title string `json:"title"`
	}
	decodeData(t, rec, &entries)
	for _, entry := range entries {
		if entry.Title == "침입" {
			t.Fatal("거부했다는 요청이 보드를 바꿨습니다")
		}
	}
}

// SSE 가 기준선 이후의 변경만 흘려보낸다.
func TestEventStream(t *testing.T) {
	s, b := newServer(t)
	// 기준선 전에 만든 것은 이벤트가 아니다.
	if _, err := b.Add(board.AddOptions{Title: "기준선 이전"}); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(s)
	defer server.Close()

	req, err := http.NewRequest("GET", server.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if got := resp.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		t.Fatalf("Content-Type = %q", got)
	}

	// 기준선이 잡힐 때까지 기다린 뒤 변경을 만든다.
	time.Sleep(100 * time.Millisecond)
	created, err := b.Add(board.AddOptions{Title: "기준선 이후"})
	if err != nil {
		t.Fatal(err)
	}

	type event struct {
		Ref  string `json:"ref"`
		ID   string `json:"id"`
		Kind string `json:"kind"`
	}
	found := make(chan event, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			payload, ok := strings.CutPrefix(scanner.Text(), "data: ")
			if !ok {
				continue
			}
			var e event
			if json.Unmarshal([]byte(payload), &e) == nil && e.ID == created.ID {
				found <- e
				return
			}
		}
	}()

	select {
	case e := <-found:
		if e.Kind != "created" {
			t.Fatalf("kind = %q, want created", e.Kind)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("변경이 스트림에 나타나지 않았습니다")
	}
}

// 루프백 판정은 인증이 없는 서버의 유일한 방어선이다.
func TestLoopback(t *testing.T) {
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:0", true},
		{"localhost:8080", true},
		{"[::1]:8080", true},
		{"0.0.0.0:8080", false},
		{"192.168.1.10:8080", false},
		{":8080", true},
		{"쓰레기", false},
	} {
		if got := Loopback(tc.addr); got != tc.want {
			t.Errorf("Loopback(%q) = %v, want %v", tc.addr, got, tc.want)
		}
	}
}
