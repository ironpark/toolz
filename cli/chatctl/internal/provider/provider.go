// Package provider는 웹 채팅 서비스별 대화 목록 수집 방법을 정의합니다.
package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
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
	// BaseURL은 서비스의 원점(origin)입니다.
	BaseURL string
	// convPath는 대화 ID 앞에 붙는 경로입니다. 예: /c/, /chat/, /app/
	convPath string
	// selector는 대화 링크를 고르는 CSS 셀렉터입니다.
	selector string
	// morePattern은 "더 보기" 류 버튼의 텍스트 패턴입니다. 비어 있으면 스크롤만 합니다.
	morePattern string
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

// expandScript는 지연 로딩되는 목록을 끝까지 펼칩니다.
//
// 세 서비스가 각각 다른 방식으로 목록을 늘립니다.
//   - ChatGPT: 사이드바를 아래로 스크롤해야 다음 묶음을 불러옵니다.
//   - Claude:  /recents 표를 아래로 스크롤합니다.
//   - Gemini:  "더보기" 버튼을 눌러야 합니다.
//
// 그래서 매 회차마다 (1) 더보기 버튼 클릭, (2) 목록의 스크롤 컨테이너를 바닥까지 내리기를
// 함께 시도하고, 개수가 두 번 연속 그대로면 끝난 것으로 봅니다.
// target 이 0보다 크면 그만큼 모이는 즉시 멈춥니다.
func expandScript(selector, morePattern string, target int, budget time.Duration) string {
	return fmt.Sprintf(`(async () => {
  const sel = %q;
  const morePattern = %q;
  const target = %d;
  const deadline = Date.now() + %d;
  const count = () => document.querySelectorAll(sel).length;

  // 목록을 담고 있는 스크롤 컨테이너를 찾습니다. 없으면 문서 자체를 씁니다.
  const scroller = (el) => {
    for (let n = el && el.parentElement; n && n !== document.body; n = n.parentElement) {
      const style = getComputedStyle(n);
      if (/(auto|scroll)/.test(style.overflowY) && n.scrollHeight > n.clientHeight + 8) return n;
    }
    return null;
  };

  let stable = 0;
  let previous = count();
  while (Date.now() < deadline && stable < 2) {
    if (target > 0 && count() >= target) break;

    if (morePattern) {
      const re = new RegExp(morePattern, 'i');
      for (const b of document.querySelectorAll('button, [role="button"]')) {
        if (re.test((b.innerText || '').trim())) b.click();
      }
    }

    const items = document.querySelectorAll(sel);
    const last = items[items.length - 1];
    const box = scroller(last);
    if (box) box.scrollTop = box.scrollHeight;
    else if (last) last.scrollIntoView({ block: 'end' });
    window.scrollTo(0, document.documentElement.scrollHeight);

    await new Promise((r) => setTimeout(r, 400));

    const now = count();
    stable = now === previous ? stable + 1 : 0;
    previous = now;
  }
  return count();
})()`, selector, morePattern, target, budget.Milliseconds())
}

func countScript(selector string) string {
	return fmt.Sprintf(`document.querySelectorAll(%q).length > 0`, selector)
}

// 대화 URL 은 각각 /c/<id>, /chat/<id>, /app/<id> 형태입니다.
// (실제 로그인 세션에서 확인: chatgpt.com/c/…, claude.ai/chat/…, gemini.google.com/app/…)
const (
	chatgptSelector = `nav a[href^="/c/"]`
	claudeSelector  = `a[href^="/chat/"]`
	geminiSelector  = `a[href^="/app/"]`
)

// All은 지원하는 모든 서비스입니다.
var All = []*Provider{
	{
		Name:     "chatgpt",
		HomeURL:  "https://chatgpt.com/",
		BaseURL:  "https://chatgpt.com",
		convPath: "/c/",
		selector: chatgptSelector,
	},
	{
		Name: "claude",
		// /recents 는 사이드바의 "모든 대화 보기" 가 여는 전체 목록 페이지입니다.
		HomeURL:  "https://claude.ai/recents",
		BaseURL:  "https://claude.ai",
		convPath: "/chat/",
		selector: claudeSelector,
	},
	{
		Name:        "gemini",
		HomeURL:     "https://gemini.google.com/app",
		BaseURL:     "https://gemini.google.com",
		convPath:    "/app/",
		selector:    geminiSelector,
		morePattern: `^(더보기|더 보기|show more)$`,
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
// 목록이 지연 로딩되므로 limit 만큼(0이면 끝까지) 펼친 뒤 읽습니다.
// 로그인되어 있지 않으면 목록이 비어 있는 상태로 대기 시간이 지나 오류를 반환합니다.
func (p *Provider) List(ctx context.Context, limit int, wait time.Duration) ([]Conversation, error) {
	// 첫 화면이 뜰 때까지 절반, 목록을 펼치는 데 나머지 절반을 씁니다.
	ready, expand := wait/2, wait/2

	var raw string
	var loaded int
	err := chromedp.Run(ctx,
		chromedp.Navigate(p.HomeURL),
		chromedp.Poll(countScript(p.selector), nil, chromedp.WithPollingTimeout(ready)),
		chromedp.Evaluate(expandScript(p.selector, p.morePattern, limit, expand), &loaded, awaitPromise),
		chromedp.Evaluate(linkScript(p.selector), &raw),
	)
	if err != nil {
		return nil, fmt.Errorf("%s 대화 목록을 읽지 못했습니다 (로그인이 필요할 수 있습니다): %w", p.Name, err)
	}
	return p.parse(raw)
}

// ConversationURL은 대화 ID 를 여는 주소를 만듭니다. 이미 URL 이면 그대로 돌려줍니다.
func (p *Provider) ConversationURL(idOrURL string) string {
	if strings.HasPrefix(idOrURL, "http") {
		return idOrURL
	}
	return p.BaseURL + p.convPath + strings.TrimPrefix(conversationID(idOrURL), "/")
}

// awaitPromise는 async 스크립트의 결과를 기다리게 합니다.
func awaitPromise(params *runtime.EvaluateParams) *runtime.EvaluateParams {
	return params.WithAwaitPromise(true)
}

// conversationID는 대화 URL 의 마지막 경로 조각을 ID 로 씁니다.
// Gemini 처럼 `?hl=ko` 가 붙는 주소도 있으므로 쿼리는 떼어냅니다.
func conversationID(rawURL string) string {
	path := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		path = u.Path
	}
	path = strings.TrimSuffix(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
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
		title := it.Title
		if title == "" {
			title = "(제목 없음)"
		}
		convs = append(convs, Conversation{
			Provider: p.Name,
			ID:       conversationID(it.URL),
			Title:    title,
			URL:      it.URL,
		})
	}
	return convs, nil
}
