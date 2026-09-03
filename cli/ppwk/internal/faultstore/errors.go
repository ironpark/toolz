package faultstore

import "errors"

// ErrAborted 는 CAS 직전에 프로세스가 죽은 상황을 흉내 낸 오류다.
var ErrAborted = errors.New("faultstore: CAS 직전 중단")
