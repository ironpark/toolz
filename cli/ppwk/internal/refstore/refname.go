package refstore

import (
	"fmt"
	"strings"
)

// ValidateRefName 은 ref 이름이 git check-ref-format 을 통과할지 검사한다.
//
// 보수적으로 판단한다. 우리가 통과시킨 이름은 git 도 반드시 통과시켜야 하며,
// 반대 방향(git 은 받는데 우리가 거부)은 허용된다. FuzzRefName 이 이 방향을 지킨다.
func ValidateRefName(ref string) error {
	if ref == "" {
		return fmt.Errorf("ref 이름이 비어 있습니다")
	}
	// git 은 슬래시가 없는 ref 도 받지만, 우리가 만드는 것은 전부 refs/ 아래다.
	//
	// 이 제약은 argv 안전장치이기도 하다. ref 이름을 git 에 인자로 넘기므로
	// "-" 로 시작하는 이름은 플래그로 해석된다. refs/ 를 강제하면 그 경로가 막힌다.
	if !strings.HasPrefix(ref, "refs/") {
		return fmt.Errorf("ref 이름이 %q 로 시작하지 않습니다: %q", "refs/", ref)
	}
	if strings.HasPrefix(ref, "/") || strings.HasSuffix(ref, "/") || strings.Contains(ref, "//") {
		return fmt.Errorf("ref 이름의 컴포넌트가 비어 있습니다: %q", ref)
	}
	if strings.Contains(ref, "..") {
		return fmt.Errorf("ref 이름에 %q 가 있습니다: %q", "..", ref)
	}
	if strings.Contains(ref, "@{") {
		return fmt.Errorf("ref 이름에 %q 가 있습니다: %q", "@{", ref)
	}
	if ref == "@" {
		return fmt.Errorf("ref 이름이 %q 입니다", "@")
	}
	if strings.HasSuffix(ref, ".") {
		return fmt.Errorf("ref 이름이 %q 로 끝납니다: %q", ".", ref)
	}
	for _, r := range ref {
		switch {
		case r < 0x20, r == 0x7f:
			return fmt.Errorf("ref 이름에 제어문자가 있습니다: %q", ref)
		case strings.ContainsRune(" ~^:?*[\\", r):
			return fmt.Errorf("ref 이름에 금지 문자 %q 가 있습니다: %q", r, ref)
		}
	}
	for _, part := range strings.Split(ref, "/") {
		if strings.HasPrefix(part, ".") {
			return fmt.Errorf("ref 컴포넌트가 %q 로 시작합니다: %q", ".", ref)
		}
		if strings.HasPrefix(part, "-") {
			return fmt.Errorf("ref 컴포넌트가 %q 로 시작합니다: %q", "-", ref)
		}
		if strings.HasSuffix(part, ".lock") {
			return fmt.Errorf("ref 컴포넌트가 %q 로 끝납니다: %q", ".lock", ref)
		}
	}
	return nil
}

// ValidateID 는 이슈·plan·결정 ID 를 검사한다 (design §3.2).
//
// ID 는 ref 이름의 마지막 컴포넌트가 되므로 [A-Za-z0-9_-]+ 로 제한한다.
// 선두 "-" 는 거부한다 — ref 이름은 git 에 인자로 넘어가므로 플래그로 읽힌다.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("ID 가 비어 있습니다")
	}
	if strings.HasPrefix(id, "-") {
		return fmt.Errorf("ID 가 %q 로 시작합니다: %q", "-", id)
	}
	for _, r := range id {
		ok := r == '-' || r == '_' ||
			(r >= '0' && r <= '9') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= 'a' && r <= 'z')
		if !ok {
			return fmt.Errorf("ID 에 쓸 수 없는 문자 %q 가 있습니다: %q", r, id)
		}
	}
	return nil
}
