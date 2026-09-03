package refstore

// ref 네임스페이스 (design §3.1).
//
// phases/ 는 없다. phase 는 독립적으로 claim 되지 않고 수명이 plan 에 종속되므로
// plan 문서 안의 배열이다. agents/ 도 없다. 에이전트 생존은 런타임 신호이며
// $GIT_COMMON_DIR/ppwk/locks/ 의 파일로 관리한다 (D13).
const (
	Prefix    = "refs/ppwk/"
	Issues    = Prefix + "issues/"
	Plans     = Prefix + "plans/"
	Decisions = Prefix + "decisions/"
	Archive   = Prefix + "archive/"
	Schema    = Prefix + "meta/schema"
)
