package gitobj

import (
	"reflect"
	"strings"
	"testing"
)

// T1.3 commit message 조립 → 파싱 왕복
func TestMessageRoundTrip(t *testing.T) {
	subject := "claim: SQLite storage 구현"
	trailers := []Trailer{
		{Key: KeyStatus, Value: "working"},
		{Key: KeyOwner, Value: "agent-b"},
		{Key: KeyPriority, Value: "high"},
		{Key: KeyDependsOn, Value: "T000"},
		{Key: KeyAgentSession, Value: "8f3a2c1d"},
	}

	message, err := BuildMessage(subject, trailers)
	if err != nil {
		t.Fatalf("BuildMessage() = %v", err)
	}
	gotSubject, gotTrailers := ParseMessage(message)
	if gotSubject != subject {
		t.Fatalf("subject = %q, want %q", gotSubject, subject)
	}
	if !reflect.DeepEqual(gotTrailers, trailers) {
		t.Fatalf("trailers = %v, want %v", gotTrailers, trailers)
	}
}

func TestMessageWithoutTrailers(t *testing.T) {
	message, err := BuildMessage("create: x", nil)
	if err != nil {
		t.Fatalf("BuildMessage() = %v", err)
	}
	subject, trailers := ParseMessage(message)
	if subject != "create: x" || len(trailers) != 0 {
		t.Fatalf("ParseMessage() = %q, %v", subject, trailers)
	}
}

// T1.4 trailer 값에 콜론/개행 포함 → 정상 파싱 또는 명시적 거부
func TestTrailerValueEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "콜론 포함", value: "https://example.com/a:b"},
		{name: "콜론과 공백", value: "Status: 처럼 보이는 값"},
		{name: "개행 포함", value: "a\nStatus: done", wantErr: true},
		{name: "캐리지리턴", value: "a\rb", wantErr: true},
		{name: "앞뒤 공백", value: " a ", wantErr: true},
		{name: "빈 값", value: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message, err := BuildMessage("create: x", []Trailer{{Key: KeyOwner, Value: tt.value}})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("BuildMessage() = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildMessage() = %v", err)
			}
			_, trailers := ParseMessage(message)
			if len(trailers) != 1 || trailers[0].Value != tt.value {
				t.Fatalf("trailers = %v, want 값 %q", trailers, tt.value)
			}
		})
	}
}

// 제목이 trailer 처럼 보여도 trailer 블록을 오염시키지 않는다.
func TestTrailerLookalikeSubject(t *testing.T) {
	subject := "create: Status: done 이라고 적힌 제목"
	message, err := BuildMessage(subject, []Trailer{{Key: KeyStatus, Value: "open"}})
	if err != nil {
		t.Fatalf("BuildMessage() = %v", err)
	}
	gotSubject, trailers := ParseMessage(message)
	if gotSubject != subject {
		t.Fatalf("subject = %q, want %q", gotSubject, subject)
	}
	if len(trailers) != 1 || trailers[0].Value != "open" {
		t.Fatalf("trailers = %v, want Status: open 하나", trailers)
	}
}

// 개행이 든 subject 는 거부한다. 호출자가 먼저 나눠야 한다.
func TestBuildMessageRejectsMultilineSubject(t *testing.T) {
	if _, err := BuildMessage("a\nStatus: done", nil); err == nil {
		t.Fatal("BuildMessage() = nil, want error")
	}
	if _, err := BuildMessage("", nil); err == nil {
		t.Fatal("BuildMessage() = nil, want error")
	}
}

func TestBuildMessageRejectsBadKey(t *testing.T) {
	for _, key := range []string{"", "has space", "1leading", "colon:key"} {
		if _, err := BuildMessage("x", []Trailer{{Key: key, Value: "v"}}); err == nil {
			t.Fatalf("BuildMessage(key=%q) = nil, want error", key)
		}
	}
}

// F1.1 만들어낸 메시지는 반드시 파싱되고 값이 왕복한다.
func FuzzMessageRoundTrip(f *testing.F) {
	f.Add("create: x", "Status", "open")
	f.Add("claim: 한글 제목", "Agent-Session", "8f3a2c1d")
	f.Add("x", "Status", "a: b")
	f.Add("Status: done", "Owner", "agent")

	f.Fuzz(func(t *testing.T, subject, key, value string) {
		trailers := []Trailer{{Key: key, Value: value}}
		message, err := BuildMessage(subject, trailers)
		if err != nil {
			return // 거부한 입력은 계약 밖이다.
		}
		gotSubject, gotTrailers := ParseMessage(message)
		if gotSubject != subject {
			t.Fatalf("subject = %q, want %q\n메시지:\n%s", gotSubject, subject, message)
		}
		if !reflect.DeepEqual(gotTrailers, trailers) {
			t.Fatalf("trailers = %v, want %v\n메시지:\n%s", gotTrailers, trailers, message)
		}
	})
}

// F1.3 임의 바이트를 파싱해도 panic 하지 않는다.
func FuzzTrailerParse(f *testing.F) {
	f.Add("create: x\n\nStatus: open\n")
	f.Add("")
	f.Add("\n\n\n")
	f.Add("a\n\nb\n\nc")

	f.Fuzz(func(t *testing.T, message string) {
		subject, trailers := ParseMessage(message)
		// 계약: subject 는 첫 줄을 넘지 않는다.
		if strings.Contains(subject, "\n") {
			t.Fatalf("subject 에 개행이 있습니다: %q", subject)
		}
		for _, tr := range trailers {
			if strings.Contains(tr.Key, "\n") || strings.Contains(tr.Value, "\n") {
				t.Fatalf("trailer 에 개행이 있습니다: %v", tr)
			}
		}
	})
}
