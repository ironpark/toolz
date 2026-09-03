package session

import (
	"crypto/rand"
	"encoding/hex"
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
