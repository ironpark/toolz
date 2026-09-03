package board

import (
	"math/rand/v2"
	"time"
)

// Backoff 는 CAS 재시도 사이의 대기 정책이다 (§4.1 6단계).
//
// jitter 가 정책의 핵심이다. N개 프로세스가 같은 ref 에서 밀리면 전부 같은
// 순간에 깨어나 다시 부딪힌다. 대기 시간을 흩뿌려야 라이브락에서 빠져나온다.
type Backoff struct {
	// Base 는 첫 대기의 상한이다. 시도마다 두 배가 된다.
	Base time.Duration
	// Max 는 대기의 절대 상한이다.
	Max time.Duration
	// LockAttempts 는 잠금 실패에 대한 재시도 상한이다.
	//
	// 잠금 실패는 상태가 안 바뀌었다는 뜻이므로 (§4.2) 같은 commit 으로 다시
	// 시도하면 된다. 재계산이 필요 없어 상한을 넉넉히 둔다.
	LockAttempts int
	// CASAttempts 는 경쟁 패배에 대한 시도 상한이다 (1 이면 재시도 없음).
	//
	// 기본은 1 이다. 경쟁에서 진 것은 재시도할 일이 아니라 다른 작업을 찾을
	// 신호이기 때문이다 (features §3). 상한에 닿으면 exit 4 로 명시적으로
	// 실패하고, 재시도할지는 호출하는 쪽이 --retry 로 정한다.
	CASAttempts int
	// Sleep 은 대기 구현이다. nil 이면 time.Sleep. 테스트가 여기를 가로챈다.
	Sleep func(time.Duration)
	// Rand 는 jitter 원천이다. nil 이면 전역 난수. 테스트가 seed 를 고정한다.
	Rand *rand.Rand
}

// DefaultBackoff 는 실제 실행에 쓰는 정책이다.
//
// core.filesRefLockTimeout 을 init 에서 1000ms 로 올려두었으므로 (§4.2) git
// 자체가 이미 상당 시간 기다린다. 여기 대기는 그 위에 얹는 것이라 짧게 둔다.
func DefaultBackoff() Backoff {
	return Backoff{
		Base:         5 * time.Millisecond,
		Max:          200 * time.Millisecond,
		LockAttempts: 12,
		CASAttempts:  1,
	}
}

// Wait 는 attempt 회차(0부터)에 해당하는 시간만큼 쉰다.
func (b Backoff) Wait(attempt int) {
	d := b.Duration(attempt)
	if d <= 0 {
		return
	}
	sleep := b.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	sleep(d)
}

// Duration 은 attempt 회차의 대기 시간이다.
//
// [0, base*2^attempt) 에서 균등하게 뽑는다. full jitter 라 부르는 방식이며,
// 하한을 두지 않는 덕분에 동시에 밀린 프로세스들이 가장 넓게 흩어진다.
func (b Backoff) Duration(attempt int) time.Duration {
	if b.Base <= 0 {
		return 0
	}
	window := b.Base
	for range attempt {
		window *= 2
		if b.Max > 0 && window >= b.Max {
			window = b.Max
			break
		}
	}
	if b.Rand != nil {
		return time.Duration(b.Rand.Int64N(int64(window)))
	}
	return time.Duration(rand.Int64N(int64(window)))
}
