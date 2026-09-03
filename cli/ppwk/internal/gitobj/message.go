// Package gitobj 는 보드 문서를 git 객체로 쓰고 읽는다 (design §14.7).
package gitobj

import (
	"fmt"
	"strings"
)

// Trailer 는 commit message 끝 블록의 한 줄이다 (§3.3).
//
// 상태를 여기에 복제해 두면 목록 조회가 for-each-ref 한 번으로 끝난다.
// issue.json 이 진실이고 trailer 는 인덱스다.
type Trailer struct {
	Key   string
	Value string
}

// trailer 키. list 가 이 이름으로 읽는다 (§5.1).
const (
	KeyStatus       = "Status"
	KeyOwner        = "Owner"
	KeyPriority     = "Priority"
	KeyPlan         = "Plan"
	KeyPhase        = "Phase"
	KeySeq          = "Seq"
	KeyDependsOn    = "Depends-On"
	KeyPhases       = "Phases"
	KeyAgentSession = "Agent-Session"
)

// BuildMessage 는 subject 와 trailer 블록을 조립한다.
//
// 규칙을 어기면 만들지 않는다. "만들어낸 메시지는 반드시 파싱된다" 가 이 함수의
// 계약이며 (F1.1), 조용히 뭉개는 것보다 거부하는 편이 낫다. 제목에
// "\nStatus: done" 같은 것이 섞여 trailer 를 오염시키는 경로를 여기서 막는다.
func BuildMessage(subject string, trailers []Trailer) (string, error) {
	if err := validateLine("subject", subject); err != nil {
		return "", err
	}
	if subject == "" {
		return "", fmt.Errorf("subject 가 비어 있습니다")
	}

	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n")
	if len(trailers) == 0 {
		return b.String(), nil
	}
	b.WriteString("\n")
	for _, t := range trailers {
		if err := validateKey(t.Key); err != nil {
			return "", err
		}
		if err := validateLine("trailer 값", t.Value); err != nil {
			return "", err
		}
		if t.Value == "" {
			return "", fmt.Errorf("trailer %q 의 값이 비어 있습니다", t.Key)
		}
		b.WriteString(t.Key)
		b.WriteString(": ")
		b.WriteString(t.Value)
		b.WriteString("\n")
	}
	return b.String(), nil
}

// ParseMessage 는 subject 와 trailer 블록을 되읽는다.
//
// 임의 바이트를 넣어도 panic 하지 않는다 (F1.3). 형태가 아니면 trailer 가 없는
// 것으로 본다.
func ParseMessage(message string) (subject string, trailers []Trailer) {
	lines := strings.Split(strings.TrimRight(message, "\n"), "\n")
	if len(lines) == 0 {
		return "", nil
	}
	subject = lines[0]
	if len(lines) < 3 || lines[1] != "" {
		return subject, nil
	}

	// 마지막 문단만 trailer 블록이다.
	block := lines[2:]
	for _, line := range block {
		key, value, found := strings.Cut(line, ":")
		if !found || validateKey(key) != nil {
			// 한 줄이라도 형태가 아니면 trailer 블록이 아니다.
			return subject, nil
		}
		trailers = append(trailers, Trailer{Key: key, Value: strings.TrimPrefix(value, " ")})
	}
	return subject, trailers
}

// TrailerValue 는 key 에 해당하는 첫 값을 돌려준다.
func TrailerValue(trailers []Trailer, key string) string {
	for _, t := range trailers {
		if t.Key == key {
			return t.Value
		}
	}
	return ""
}

// validateKey 는 trailer 키가 [A-Za-z][A-Za-z0-9-]* 인지 본다.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("trailer 키가 비어 있습니다")
	}
	for i, r := range key {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z')
		if i > 0 {
			ok = ok || r == '-' || (r >= '0' && r <= '9')
		}
		if !ok {
			return fmt.Errorf("trailer 키에 쓸 수 없는 문자 %q 가 있습니다: %q", r, key)
		}
	}
	return nil
}

// validateLine 은 한 줄에 들어갈 수 있는 값인지 본다.
//
// 개행과 제어문자를 막고, 앞뒤 공백도 막는다. 파싱이 값을 다듬으면 왕복이
// 깨지므로 애초에 다듬을 것이 없는 값만 받는다.
func validateLine(what, s string) error {
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s 에 제어문자가 있습니다: %q", what, s)
		}
	}
	if strings.TrimSpace(s) != s {
		return fmt.Errorf("%s 의 앞뒤에 공백이 있습니다: %q", what, s)
	}
	return nil
}
