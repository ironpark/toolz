package model

import (
	"bytes"
	"encoding/json"
)

// Marshal 은 문서를 JSON 으로 낸다.
//
// encoding/json 의 기본 HTML 이스케이프를 끈다. 켜져 있으면 보존한 미지 필드의
// 원본 바이트가 다시 쓰이면서("&" → "&") 왕복이 깨진다. 우리가 이해하지
// 못하는 값을 다시 써내는 것 자체가 잘못이다 (§9.4).
//
// 저장되는 문서는 전부 이 함수를 거쳐야 한다. json.Marshal 을 직접 부르면
// 이스케이프가 되살아난다.
func Marshal(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
