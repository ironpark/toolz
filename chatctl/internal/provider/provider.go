// Package provider는 웹 채팅 서비스별 대화 목록 수집 방법을 정의합니다.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// Conversation은 하나의 대화 스레드를 나타냅니다.
type Conversation struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	URL      string `json:"url"`
}

// Provider는 하나의 채팅 서비스입니다.
type Provider struct {
	// Name은 CLI에서 사용하는 식별자입니다.
	Name string
	// HomeURL은 로그인 및 대화 목록이 보이는 주소입니다.
	HomeURL string
	// listScript는 사이드바에서 대화 목록을 추출하는 JS 표현식입니다.
	listScript string
	// readyScript는 목록이 로드되었는지 판별하는 JS 표현식입니다.
	readyScript string
}

// 사이드바 링크에서 대화 목록을 뽑아내는 공통 스크립트를 만듭니다.
// selector에 해당하는 <a> 요소의 href와 텍스트를 수집합니다.
func linkScript(selector string) string {
	return fmt.Sprintf(`(() => {
  const seen = new Set();
  const out = [];
  for (const a of document.querySelectorAll(%q)) {
    const href = a.href;
    if (!href || seen.has(href)) continue;
    seen.add(href);
    const title = (a.getAttribute('title') || a.innerText || '').trim().split('\n')[0];
    out.push({ url: href, title: title });
  }
  return JSON.stringify(out);
})()`, selector)
}

func countScript(selector string) string {
	return fmt.Sprintf(`document.querySelectorAll(%q).length > 0`, selector)
}

const (
	chatgptSelector = `nav a[href^="/c/"]`
	claudeSelector  = `a[href^="/chat/"]`
	geminiSelector  = `[data-test-id="conversation"], .conversation-items-container a`
)

// All은 지원하는 모든 서비스입니다.
var All = []*Provider{
	{
		Name:        "chatgpt",
		HomeURL:     "https://chatgpt.com/",
		listScript:  linkScript(chatgptSelector),
		readyScript: countScript(chatgptSelector),
	},
	{
		Name:        "claude",
		HomeURL:     "https://claude.ai/recents",
		listScript:  linkScript(claudeSelector),
		readyScript: countScript(claudeSelector),
	},
	{
		Name:        "gemini",
		HomeURL:     "https://gemini.google.com/app",
		listScript:  linkScript(geminiSelector),
		readyScript: countScript(geminiSelector),
	},
}

// Names는 지원하는 서비스 이름 목록입니다.
func Names() []string {
	names := make([]string, 0, len(All))
	for _, p := range All {
		names = append(names, p.Name)
	}
	sort.Strings(names)
	return names
}

// Get은 이름으로 서비스를 찾습니다.
func Get(name string) (*Provider, error) {
	for _, p := range All {
		if p.Name == strings.ToLower(name) {
			return p, nil
		}
	}
	return nil, fmt.Errorf("알 수 없는 서비스 %q (사용 가능: %s)", name, strings.Join(Names(), ", "))
}

// List는 이미 열린 브라우저 컨텍스트에서 대화 목록을 수집합니다.
// 로그인되어 있지 않으면 목록이 비어 있는 상태로 대기 시간이 지나 오류를 반환합니다.
func (p *Provider) List(ctx context.Context, wait time.Duration) ([]Conversation, error) {
	var raw string
	err := chromedp.Run(ctx,
		chromedp.Navigate(p.HomeURL),
		chromedp.Poll(p.readyScript, nil, chromedp.WithPollingTimeout(wait)),
		chromedp.Evaluate(p.listScript, &raw),
	)
	if err != nil {
		return nil, fmt.Errorf("%s 대화 목록을 읽지 못했습니다 (로그인이 필요할 수 있습니다): %w", p.Name, err)
	}
	return p.parse(raw)
}

func (p *Provider) parse(raw string) ([]Conversation, error) {
	var items []struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("%s 응답 파싱 실패: %w", p.Name, err)
	}

	convs := make([]Conversation, 0, len(items))
	for _, it := range items {
		id := it.URL
		if idx := strings.LastIndex(strings.TrimSuffix(it.URL, "/"), "/"); idx >= 0 {
			id = strings.TrimSuffix(it.URL, "/")[idx+1:]
		}
		title := it.Title
		if title == "" {
			title = "(제목 없음)"
		}
		convs = append(convs, Conversation{
			Provider: p.Name,
			ID:       id,
			Title:    title,
			URL:      it.URL,
		})
	}
	return convs, nil
}
