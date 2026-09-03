package model

// Lease 는 잠금 파일의 내용이다 (design §3.6).
//
// $GIT_COMMON_DIR/ppwk/locks/ 아래 있으므로 모든 worktree 가 읽는다.
// 별도 ref 를 두지 않는다 (D13).
type Lease struct {
	Agent    string    `json:"agent"`
	Session  string    `json:"session"`
	Worktree string    `json:"worktree"`
	Since    Timestamp `json:"since"`
	// LastActivity 는 상태 변경 명령만 갱신한다. 읽기는 쓰지 않는다.
	LastActivity Timestamp `json:"last_activity"`
	// HookPID 는 SessionStart 훅이 설치된 경우에만 채워진다 (§3.8).
	HookPID *int `json:"hook_pid"`
	// HookStarttime 은 pid 재사용을 걸러낸다.
	HookStarttime *string `json:"hook_starttime"`
}
