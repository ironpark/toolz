package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// 스크롤해야 다음 묶음이 붙는 사이드바 (ChatGPT/Claude 방식)
const scrollPage = `<html><body>
<div id="side" style="height:200px;overflow-y:auto"><div id="list"></div></div>
<script>
let n=0;
function add(k){for(let i=0;i<k;i++){n++;const a=document.createElement('a');a.href='/c/id'+n;a.textContent='chat '+n;a.style.display='block';a.style.height='40px';document.getElementById('list').appendChild(a);}}
add(20);
const side=document.getElementById('side');
side.addEventListener('scroll',()=>{if(side.scrollTop+side.clientHeight>=side.scrollHeight-5&&n<75){setTimeout(()=>add(20),100);}});
</script></body></html>`

// "더보기" 버튼을 눌러야 늘어나는 목록 (Gemini 방식)
const buttonPage = `<html><body>
<div id="list"></div><button id="more">더보기</button>
<script>
let n=0;
function add(k){for(let i=0;i<k;i++){n++;const a=document.createElement('a');a.href='/c/id'+n;a.textContent='chat '+n;a.style.display='block';document.getElementById('list').appendChild(a);}}
add(10);
document.getElementById('more').addEventListener('click',()=>{if(n<50)add(10);if(n>=50)document.getElementById('more').remove();});
</script></body></html>`

func newTestProvider(homeURL, morePattern string) *Provider {
	const sel = `a[href^="/c/"]`
	return &Provider{
		Name: "test", HomeURL: homeURL, BaseURL: homeURL, convPath: "/c/",
		selector: sel, morePattern: morePattern,
		listScript: linkScript(sel), readyScript: countScript(sel),
	}
}

func browserContext(t *testing.T) context.Context {
	t.Helper()
	alloc, cancelAlloc := chromedp.NewExecAllocator(context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.Flag("headless", true))...)
	ctx, cancelCtx := chromedp.NewContext(alloc)
	if err := chromedp.Run(ctx); err != nil {
		cancelCtx()
		cancelAlloc()
		t.Skipf("Chrome 을 실행할 수 없어 건너뜁니다: %v", err)
	}
	t.Cleanup(func() { cancelCtx(); cancelAlloc() })
	return ctx
}

func serve(t *testing.T, html string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, html)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestListExpandsLazyLists(t *testing.T) {
	if testing.Short() {
		t.Skip("브라우저가 필요한 테스트")
	}
	ctx := browserContext(t)

	t.Run("스크롤로 늘어나는 목록", func(t *testing.T) {
		p := newTestProvider(serve(t, scrollPage), "")
		convs, err := p.List(ctx, 0, 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if len(convs) != 80 { // 20개씩 붙어 n<75 조건을 넘기면 80에서 멈춥니다
			t.Errorf("전체를 가져오지 못했습니다: %d개", len(convs))
		}
	})

	t.Run("limit 만큼만 펼치기", func(t *testing.T) {
		p := newTestProvider(serve(t, scrollPage), "")
		convs, err := p.List(ctx, 25, 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if len(convs) < 25 || len(convs) >= 80 {
			t.Errorf("limit 에서 멈추지 않았습니다: %d개", len(convs))
		}
	})

	t.Run("더보기 버튼", func(t *testing.T) {
		p := newTestProvider(serve(t, buttonPage), `^(더보기|더 보기|show more)$`)
		convs, err := p.List(ctx, 0, 20*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if len(convs) != 50 {
			t.Errorf("버튼을 끝까지 누르지 못했습니다: %d개", len(convs))
		}
	})
}
