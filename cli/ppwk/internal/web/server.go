package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ironpark/toolz/cli/ppwk/internal/board"
	"github.com/ironpark/toolz/cli/ppwk/internal/model"
	"github.com/ironpark/toolz/cli/ppwk/internal/watch"
)

// Server 는 보드 하나를 브라우저에 노출한다.
type Server struct {
	// mu 는 보드 접근을 직렬화한다.
	//
	// Board 는 go-git 저장소 하나를 들고 있어 동시 사용에 안전하지 않다.
	// HTTP 핸들러는 당연히 동시에 돈다. 요청마다 보드를 새로 여는 방법도
	// 있지만, 로컬 UI 한 사람이 쓰는 서버에서 그 비용을 매번 치를 이유가
	// 없다 — 잠금은 요청 하나가 끝날 때까지만 잡힌다.
	mu    sync.Mutex
	board *board.Board

	handler http.Handler
	// pollInterval 은 SSE 가 변경을 확인하는 주기다.
	pollInterval time.Duration
}

// Options 는 서버 설정이다.
type Options struct {
	// PollInterval 은 SSE 의 감지 주기다. 0 이면 1초다.
	PollInterval time.Duration
}

// New 는 보드를 감싸는 서버를 만든다.
func New(b *board.Board, opts Options) *Server {
	s := &Server{board: b, pollInterval: opts.PollInterval}
	if s.pollInterval <= 0 {
		// CLI 의 watch 보다 짧게 잡는다. 사람이 화면을 보고 있는 동안의
		// 2초는 길게 느껴진다.
		s.pollInterval = time.Second
	}
	s.handler = s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.handler.ServeHTTP(w, r) }

// routes 는 API 와 정적 자산을 붙인다.
func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/state", s.handle(s.getState))
	mux.HandleFunc("GET /api/issues", s.handle(s.listIssues))
	mux.HandleFunc("POST /api/issues", s.handle(s.addIssue))
	mux.HandleFunc("GET /api/issues/{id}", s.handle(s.showIssue))
	mux.HandleFunc("GET /api/issues/{id}/history", s.handle(s.issueHistory))
	mux.HandleFunc("POST /api/issues/{id}/actions/{action}", s.handle(s.transition))
	mux.HandleFunc("GET /api/next", s.handle(s.candidates))
	mux.HandleFunc("POST /api/next", s.handle(s.claimNext))
	mux.HandleFunc("GET /api/plans", s.handle(s.listPlans))
	mux.HandleFunc("GET /api/plans/{id}", s.handle(s.showPlan))
	mux.HandleFunc("GET /api/decisions", s.handle(s.listDecisions))
	mux.HandleFunc("GET /api/decisions/{id}", s.handle(s.showDecision))
	mux.HandleFunc("GET /api/agents", s.handle(s.listAgents))
	mux.HandleFunc("GET /api/doctor", s.handle(s.doctor))
	mux.HandleFunc("GET /api/fsck", s.handle(s.fsck))
	mux.HandleFunc("GET /api/events", s.events)

	// 없는 API 는 없다고 말한다. 이것이 없으면 아래 SPA 규칙이 /api/ 까지
	// 삼켜서, 오타 난 요청이 index.html 을 200 으로 받는다.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotFound, envelope{
			Error: &apiError{Kind: "not_found", Message: "없는 엔드포인트: " + r.URL.Path},
		})
	})

	// API 밖은 전부 SPA 다. 알 수 없는 경로는 index.html 로 보낸다.
	mux.Handle("/", spa(Assets()))

	return guard(mux)
}

// with 는 보드 접근을 잠금 아래에서 수행한다.
func (s *Server) with(fn func(*board.Board) (any, error)) (any, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return fn(s.board)
}

// ---- 핸들러 ----

// State 는 화면을 처음 그릴 때 필요한 것들이다.
type State struct {
	Agent    string `json:"agent"`
	Session  string `json:"session"`
	Worktree string `json:"worktree"`
	Schema   int    `json:"schema"`
	// ReadOnly 는 이 보드가 이 CLI 보다 높은 스키마인지다.
	ReadOnly bool `json:"read_only"`
}

func (s *Server) getState(r *http.Request) (any, error) {
	return s.with(func(b *board.Board) (any, error) {
		schema, err := b.SchemaVersion()
		if err != nil {
			return nil, err
		}
		id := b.Identity()
		return State{
			Agent:    id.Agent,
			Session:  id.Session,
			Worktree: b.Root(),
			Schema:   schema,
			ReadOnly: schema > model.SchemaVersion,
		}, nil
	})
}

func (s *Server) listIssues(r *http.Request) (any, error) {
	q := r.URL.Query()
	opts := board.ListOptions{
		Owner:      q.Get("owner"),
		Label:      q.Get("label"),
		Plan:       q.Get("plan"),
		Phase:      q.Get("phase"),
		Unassigned: q.Has("unassigned"),
		Mine:       q.Has("mine"),
		Archived:   q.Has("archived"),
		All:        q.Has("all"),
	}
	for _, v := range q["status"] {
		opts.Status = append(opts.Status, model.Status(v))
	}
	for _, v := range q["priority"] {
		opts.Priority = append(opts.Priority, model.Priority(v))
	}
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, badRequest("limit: %v", err)
		}
		opts.Limit = n
	}
	return s.with(func(b *board.Board) (any, error) {
		entries, err := b.List(opts)
		if err != nil {
			return nil, err
		}
		if entries == nil {
			entries = []board.ListEntry{}
		}
		return entries, nil
	})
}

// addRequest 는 이슈 생성 입력이다.
type addRequest struct {
	Title     string   `json:"title"`
	Body      string   `json:"body"`
	Priority  string   `json:"priority"`
	Labels    []string `json:"labels"`
	DependsOn []string `json:"depends_on"`
	Plan      string   `json:"plan"`
	Phase     string   `json:"phase"`
	Seq       *int     `json:"seq"`
}

func (s *Server) addIssue(r *http.Request) (any, error) {
	var req addRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, badRequest("제목이 비어 있습니다")
	}
	opts := board.AddOptions{
		Title:     req.Title,
		Body:      []byte(req.Body),
		Priority:  model.Priority(req.Priority),
		Labels:    req.Labels,
		DependsOn: req.DependsOn,
		Plan:      req.Plan,
		Phase:     req.Phase,
	}
	if req.Seq != nil {
		opts.Seq, opts.SeqSet = *req.Seq, true
	}
	return s.with(func(b *board.Board) (any, error) { return b.Add(opts) })
}

func (s *Server) showIssue(r *http.Request) (any, error) {
	id := r.PathValue("id")
	return s.with(func(b *board.Board) (any, error) {
		issue, err := b.Show(id)
		if err != nil {
			return nil, err
		}
		decisions, err := b.DecisionsForIssue(id)
		if err != nil {
			return nil, err
		}
		if decisions == nil {
			decisions = []board.DecisionEntry{}
		}
		// 본문은 별도 필드로 낸다. Issue 는 []byte 를 base64 로 내보낸다.
		return map[string]any{
			"issue":     issue.Issue,
			"body":      string(issue.Body),
			"ref":       issue.Ref,
			"commit":    issue.Commit.String(),
			"archived":  issue.Archived(),
			"decisions": decisions,
		}, nil
	})
}

func (s *Server) issueHistory(r *http.Request) (any, error) {
	id := r.PathValue("id")
	return s.with(func(b *board.Board) (any, error) {
		events, err := b.History(id, 0)
		if err != nil {
			return nil, err
		}
		if events == nil {
			events = []board.Event{}
		}
		return events, nil
	})
}

// transitionRequest 는 상태 전이 입력이다.
type transitionRequest struct {
	Message string `json:"message"`
	On      string `json:"on"`
	Force   bool   `json:"force"`
}

// allowedActions 는 웹에서 부를 수 있는 전이다.
//
// 목록을 명시한다. 경로 조각을 그대로 Action 으로 쓰면 나중에 추가된 내부용
// 전이가 의도치 않게 열린다.
var allowedActions = map[string]board.Action{
	"claim":   board.ActionClaim,
	"start":   board.ActionStart,
	"done":    board.ActionDone,
	"block":   board.ActionBlock,
	"unblock": board.ActionUnblock,
	"release": board.ActionRelease,
	"cancel":  board.ActionCancel,
}

func (s *Server) transition(r *http.Request) (any, error) {
	action, ok := allowedActions[r.PathValue("action")]
	if !ok {
		return nil, badRequest("알 수 없는 전이: %s", r.PathValue("action"))
	}
	var req transitionRequest
	if err := decode(r, &req); err != nil {
		return nil, err
	}
	id := r.PathValue("id")
	return s.with(func(b *board.Board) (any, error) {
		return b.Transition(action, id, board.TransitionOptions{
			Message: req.Message, On: req.On, Force: req.Force,
		})
	})
}

func (s *Server) candidates(r *http.Request) (any, error) {
	opts := board.NextOptions{
		Plan:   r.URL.Query().Get("plan"),
		Label:  r.URL.Query().Get("label"),
		DryRun: true,
	}
	return s.with(func(b *board.Board) (any, error) {
		issues, err := b.Candidates(opts)
		if err != nil {
			return nil, err
		}
		out := make([]model.Issue, 0, len(issues))
		for _, issue := range issues {
			out = append(out, issue.Issue)
		}
		return out, nil
	})
}

func (s *Server) claimNext(r *http.Request) (any, error) {
	q := r.URL.Query()
	return s.with(func(b *board.Board) (any, error) {
		return b.Next(board.NextOptions{
			Plan: q.Get("plan"), Label: q.Get("label"), Claim: true, MaxAttempts: 5,
		})
	})
}

func (s *Server) listPlans(r *http.Request) (any, error) {
	status := model.PlanStatus(r.URL.Query().Get("status"))
	return s.with(func(b *board.Board) (any, error) {
		plans, err := b.ListPlans(status)
		if err != nil {
			return nil, err
		}
		// 목록에도 진행률을 실어 보낸다. 화면이 plan 마다 다시 묻지 않도록.
		views := make([]*board.PlanView, 0, len(plans))
		for _, plan := range plans {
			view, err := b.ShowPlanView(plan.ID)
			if err != nil {
				return nil, err
			}
			views = append(views, view)
		}
		return views, nil
	})
}

func (s *Server) showPlan(r *http.Request) (any, error) {
	id := r.PathValue("id")
	return s.with(func(b *board.Board) (any, error) { return b.ShowPlanView(id) })
}

func (s *Server) listDecisions(r *http.Request) (any, error) {
	q := r.URL.Query()
	opts := board.DecisionListOptions{
		All: q.Has("all"), Issue: q.Get("issue"), Plan: q.Get("plan"), Search: q.Get("search"),
	}
	return s.with(func(b *board.Board) (any, error) {
		entries, err := b.ListDecisions(opts)
		if err != nil {
			return nil, err
		}
		if entries == nil {
			entries = []board.DecisionEntry{}
		}
		return entries, nil
	})
}

func (s *Server) showDecision(r *http.Request) (any, error) {
	id := r.PathValue("id")
	return s.with(func(b *board.Board) (any, error) { return b.ShowDecision(id) })
}

func (s *Server) listAgents(r *http.Request) (any, error) {
	return s.with(func(b *board.Board) (any, error) {
		leases := b.Agents()
		out := make([]map[string]any, 0, len(leases))
		for _, lease := range leases {
			out = append(out, map[string]any{
				"agent": lease.Agent, "session": lease.Session, "worktree": lease.Worktree,
				"since": lease.Since, "last_activity": lease.LastActivity,
				"hook_pid": lease.HookPID, "alive": b.LeaseAlive(lease),
			})
		}
		return out, nil
	})
}

func (s *Server) doctor(r *http.Request) (any, error) {
	return s.with(func(b *board.Board) (any, error) {
		stats, err := b.RefStats()
		if err != nil {
			return nil, err
		}
		lease, held := b.WorktreeLease()
		return map[string]any{
			"identity":     b.Identity(),
			"worktree":     b.Root(),
			"lease":        lease,
			"lease_held":   held,
			"activity_ttl": b.ActivityTTL().String(),
			"stale_locks":  b.StaleLocks(),
			"refs":         stats,
		}, nil
	})
}

func (s *Server) fsck(r *http.Request) (any, error) {
	return s.with(func(b *board.Board) (any, error) {
		findings, err := b.Fsck(board.FsckOptions{})
		if err != nil {
			return nil, err
		}
		if findings == nil {
			findings = []board.Finding{}
		}
		return findings, nil
	})
}

// events 는 ref 변경을 SSE 로 흘려보낸다.
//
// 화면이 주기적으로 전체를 다시 묻는 것보다 낫다 — 무엇이 바뀌었는지 알면
// 그 부분만 다시 읽으면 된다. polling 은 서버 안에 한 번만 있으면 된다.
func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	// Poller 는 이 연결이 들고 있는다. 스냅샷을 갖고 있어야 무엇이 바뀌었는지
	// 알 수 있고, 첫 주기는 기준선을 잡는 데 쓰인다.
	poller := s.newPoller()

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		events, err := s.poll(poller)
		if err != nil {
			return
		}
		for _, event := range events {
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", payload)
		}
		flusher.Flush()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// newPoller 는 이 보드를 보는 poller 를 만든다.
func (s *Server) newPoller() *watch.Poller {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.board.Poller(board.WatchOptions{})
}

// poll 은 한 주기를 돌린다.
func (s *Server) poll(poller *watch.Poller) ([]watch.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.board.PollOnce(poller)
}

// ---- 배관 ----

// envelope 는 CLI 의 --json 과 같은 봉투다 (features §0.4).
type envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *apiError `json:"error,omitempty"`
	OK    bool      `json:"ok"`
}

type apiError struct {
	Kind    string `json:"kind"`
	Message string `json:"message"`
}

// httpError 는 상태 코드를 동반하는 오류다.
type httpError struct {
	Status int
	Kind   string
	Msg    string
}

func (e *httpError) Error() string { return e.Msg }

func badRequest(format string, args ...any) *httpError {
	return &httpError{Status: http.StatusBadRequest, Kind: "usage", Msg: fmt.Sprintf(format, args...)}
}

// handle 은 값을 돌려주는 함수를 핸들러로 만든다.
func (s *Server) handle(fn func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := fn(r)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, envelope{Data: data, OK: true})
	}
}

// writeError 는 도메인 오류를 HTTP 상태로 옮긴다.
//
// CLI 의 종료 코드와 같은 분류를 쓰되 화폐 단위만 다르다 (§0.3). 분류를
// 핸들러마다 하면 새 핸들러가 늘 때 반드시 빠뜨린다.
func writeError(w http.ResponseWriter, err error) {
	status, kind := http.StatusInternalServerError, "internal"

	var coded *httpError
	var transition *board.TransitionError
	var conflict *board.ConflictError
	switch {
	case errors.As(err, &coded):
		status, kind = coded.Status, coded.Kind
	case errors.As(err, &transition):
		status, kind = http.StatusConflict, "invalid_transition"
	case errors.As(err, &conflict):
		status, kind = http.StatusConflict, "cas_conflict"
	case errors.Is(err, board.ErrNotFound):
		status, kind = http.StatusNotFound, "not_found"
	case errors.Is(err, board.ErrNotTerminal), errors.Is(err, board.ErrAlreadyArchived),
		errors.Is(err, board.ErrPhaseInUse):
		status, kind = http.StatusConflict, "invalid_transition"
	case errors.Is(err, board.ErrSchemaTooNew):
		status, kind = http.StatusServiceUnavailable, "schema_mismatch"
	}
	writeJSON(w, status, envelope{Error: &apiError{Kind: kind, Message: err.Error()}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

// decode 는 요청 본문을 읽는다. 비어 있으면 그대로 둔다.
func decode(r *http.Request, v any) error {
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		return badRequest("본문을 읽을 수 없습니다: %v", err)
	}
	return nil
}

// spa 는 정적 자산을 내보내고, 없는 경로는 index.html 로 보낸다.
func spa(assets fs.FS) http.Handler {
	files := http.FileServerFS(assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		if _, err := fs.Stat(assets, path); err != nil {
			// 클라이언트 라우팅이다. 없는 파일이 아니라 앱의 경로다.
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

// guard 는 브라우저에서 오는 교차 출처 요청을 막는다.
//
// 이 서버는 loopback 에 뜨지만 그것만으로는 부족하다. 사용자가 어떤 웹
// 페이지를 열어 두었든 그 페이지의 스크립트는 http://127.0.0.1:<port> 로
// 요청을 보낼 수 있고, 이 API 는 보드를 바꾼다. Origin 을 확인해 우리
// 화면에서 온 것만 받는다.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && !sameOrigin(origin, r.Host) {
			writeJSON(w, http.StatusForbidden, envelope{
				Error: &apiError{Kind: "forbidden", Message: "교차 출처 요청은 받지 않습니다"},
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func sameOrigin(origin, host string) bool {
	for _, prefix := range []string{"http://", "https://"} {
		if trimmed, found := strings.CutPrefix(origin, prefix); found {
			return trimmed == host
		}
	}
	return false
}

// Loopback 은 주소가 루프백에만 묶이는지다.
//
// 다른 주소에 묶으면 같은 네트워크의 누구나 이 보드를 바꿀 수 있다. 인증이
// 없으므로 그것은 열어 두는 것과 같다. 막지는 않고 알린다.
func Loopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "" || host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
