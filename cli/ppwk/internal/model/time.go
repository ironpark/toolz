package model

import (
	"encoding/json"
	"fmt"
	"time"
)

// Timestamp 는 RFC3339 초 단위 UTC 시각이다.
//
// 정밀도를 초로 고정하는 이유는 content-addressing 이다. 같은 내용이 같은 OID 로
// 수렴해야 하므로 직렬화 결과가 흔들리면 안 된다.
type Timestamp struct {
	time.Time
}

// Now 는 현재 시각을 초 단위로 자른다.
func Now() Timestamp {
	return Timestamp{time.Now().UTC().Truncate(time.Second)}
}

// NewTimestamp 는 t 를 초 단위 UTC 로 맞춘다.
func NewTimestamp(t time.Time) Timestamp {
	return Timestamp{t.UTC().Truncate(time.Second)}
}

func (t Timestamp) MarshalJSON() ([]byte, error) {
	return json.Marshal(t.UTC().Truncate(time.Second).Format(time.RFC3339))
}

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return fmt.Errorf("시각을 읽을 수 없습니다 (%q): %w", s, err)
	}
	t.Time = parsed.UTC().Truncate(time.Second)
	return nil
}

func (t Timestamp) String() string {
	return t.UTC().Truncate(time.Second).Format(time.RFC3339)
}
