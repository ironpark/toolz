package model

import (
	"encoding/json"
	"slices"
)

// unknownFields 는 known 에 없는 키만 골라낸다.
func unknownFields(data []byte, known []string) (map[string]json.RawMessage, error) {
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	var extra map[string]json.RawMessage
	for key, value := range all {
		if slices.Contains(known, key) {
			continue
		}
		if extra == nil {
			extra = make(map[string]json.RawMessage)
		}
		extra[key] = value
	}
	return extra, nil
}

// marshalWithExtra 는 문서와 보존된 미지 필드를 합친다.
//
// map 직렬화는 키 순서가 정렬되므로 결과가 결정적이다. content-addressing 이
// 걸려 있으므로 이 성질이 필요하다.
func marshalWithExtra(doc any, extra map[string]json.RawMessage) ([]byte, error) {
	data, err := Marshal(doc)
	if err != nil {
		return nil, err
	}
	if len(extra) == 0 {
		return data, nil
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(data, &merged); err != nil {
		return nil, err
	}
	for key, value := range extra {
		// 아는 필드가 이긴다. 미지 필드가 덮어쓰지 않는다.
		if _, taken := merged[key]; !taken {
			merged[key] = value
		}
	}
	return Marshal(merged)
}
