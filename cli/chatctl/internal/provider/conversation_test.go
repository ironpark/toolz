package provider

import "testing"

func TestConversationURL(t *testing.T) {
	cases := []struct{ prov, in, want string }{
		{"chatgpt", "6a8ebf8c-c2f0-83ee-95e0-6556de6e1394", "https://chatgpt.com/c/6a8ebf8c-c2f0-83ee-95e0-6556de6e1394"},
		{"claude", "dd83d684-cfec-4cf0-bc2d-95cdf7039c90", "https://claude.ai/chat/dd83d684-cfec-4cf0-bc2d-95cdf7039c90"},
		{"gemini", "ee88c14d4c19f088", "https://gemini.google.com/app/ee88c14d4c19f088"},
		{"gemini", "https://gemini.google.com/app/ee88c14d4c19f088?hl=ko", "https://gemini.google.com/app/ee88c14d4c19f088?hl=ko"},
	}
	for _, c := range cases {
		p, err := Get(c.prov)
		if err != nil {
			t.Fatal(err)
		}
		if got := p.ConversationURL(c.in); got != c.want {
			t.Errorf("%s: got %q want %q", c.prov, got, c.want)
		}
	}
}

func TestConversationID(t *testing.T) {
	if got := conversationID("https://gemini.google.com/app/ee88c14d4c19f088?hl=ko"); got != "ee88c14d4c19f088" {
		t.Errorf("got %q", got)
	}
}
