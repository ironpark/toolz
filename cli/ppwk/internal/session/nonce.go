package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
)

// nonceBytes 는 세션 nonce 의 길이다.
//
// 128비트를 쓰는 이유는 §4.3 이다. nonce 는 commit content 에 들어가 OID 를
// 갈라놓는 유일한 값이므로, 충돌하면 서로 다른 두 에이전트가 같은 commit 을
// 만들어 양쪽 CAS 가 모두 성공한다. 128비트면 그 확률은 무시할 수 있다.
const nonceBytes = 16

// NewNonce 는 세션 nonce 를 만든다.
//
// crypto/rand 를 쓴다. math/rand 는 프로세스가 같은 시각에 여럿 뜨는 이
// 시스템에서 시드가 겹칠 수 있고, 그 결과가 곧 OID 충돌이다.
func NewNonce() string {
	buf := make([]byte, nonceBytes)
	// Go 1.24+ 의 crypto/rand.Read 는 실패하지 않는다. 실패하면 프로그램을
	// 계속할 이유가 없으므로 panic 이 맞다.
	if _, err := rand.Read(buf); err != nil {
		panic("session: crypto/rand 를 읽을 수 없습니다: " + err.Error())
	}
	return hex.EncodeToString(buf)
}

// ShellSession 은 도구가 감지되지 않을 때 쓰는 세션 ID 다.
//
// nonce 를 그때그때 만들면 명령마다 다른 세션이 되고, 그러면 claim 다음의
// start 가 자기 것이 아니게 되며 list --mine 도 늘 비어 있게 된다. 셸에서
// 쓰는 사람에게는 그것이 곧 "아무것도 안 되는" 상태다.
//
// 부모 프로세스를 세션으로 삼는다. 같은 셸에서 실행한 명령들은 같은 PPID 를
// 갖고, 다른 터미널은 다른 PPID 를 가지며, 셸이 죽으면 그 세션도 끝난다 —
// 세션이라는 말의 뜻 그대로다. 훅 경로가 부모를 도구로 보는 것과 같은 논리다.
//
// 시작 시각을 함께 넣는 이유는 PID 재사용 때문이다. 넣지 않으면 오래전에
// 죽은 셸과 지금 셸이 같은 세션으로 묶일 수 있다.
func ShellSession() (id string, ok bool) {
	ppid := os.Getppid()
	if ppid <= 1 {
		return "", false
	}
	start := ProcessStarttime(ppid)
	if start == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(strconv.Itoa(ppid) + "\x00" + start))
	return "shell-" + hex.EncodeToString(sum[:8]), true
}
