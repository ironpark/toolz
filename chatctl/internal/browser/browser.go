// Package browser는 chromedp 세션을 열고 프로필 디렉터리를 관리합니다.
//
// 엔진은 두 가지입니다.
//   - moli:   github.com/lexmount/moli 의 `moli serve` 를 띄우고 CDP 로 붙습니다. 헤드리스 전용입니다.
//   - chrome: 로컬 Chrome / Chromium 을 직접 실행합니다. 로그인처럼 창이 필요할 때 씁니다.
//
// 호출하는 쪽은 창이 필요한지(Headless)만 정하고, 어떤 엔진을 쓸지는 Resolve 가 판단합니다.
package browser

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/ironpark/toolz/chatctl/internal/session"
)

// Engine은 사용할 브라우저 실행 방식입니다.
type Engine string

const (
	// EngineAuto는 moli 가 설치되어 있으면 moli, 아니면 chrome 을 씁니다.
	EngineAuto Engine = "auto"
	// EngineMoli는 moli serve 에 CDP 로 접속합니다.
	EngineMoli Engine = "moli"
	// EngineChrome은 로컬 Chrome 을 직접 실행합니다.
	EngineChrome Engine = "chrome"
)

// ParseEngine은 문자열을 Engine 으로 바꿉니다.
func ParseEngine(s string) (Engine, error) {
	switch Engine(s) {
	case EngineAuto, EngineMoli, EngineChrome:
		return Engine(s), nil
	}
	return "", fmt.Errorf("알 수 없는 엔진 %q (사용 가능: %s, %s, %s)", s, EngineAuto, EngineMoli, EngineChrome)
}

// MoliPath는 설치된 moli 실행 파일 경로를 반환합니다.
func MoliPath() (string, bool) {
	path, err := exec.LookPath("moli")
	if err != nil {
		return "", false
	}
	return path, true
}

// Options는 브라우저 실행 방식을 정의합니다.
type Options struct {
	// Engine은 사용할 엔진입니다. 비어 있으면 EngineAuto 로 취급합니다.
	Engine Engine
	// Profile은 로그인 세션을 보관할 프로필 이름입니다.
	Profile string
	// Headless가 false이면 창을 띄웁니다. 로그인처럼 사람이 봐야 하는 작업에 필요합니다.
	// moli 는 창을 띄울 수 없으므로 auto 는 이때 chrome 을 고릅니다.
	Headless bool
	// Timeout은 전체 작업 제한 시간입니다.
	Timeout time.Duration
}

// Session은 열린 브라우저 컨텍스트입니다.
type Session struct {
	// Ctx는 chromedp 작업에 쓰는 컨텍스트입니다.
	Ctx context.Context
	// Engine은 실제로 선택된 엔진입니다.
	Engine Engine
	// Dir은 이 세션이 쓰는 프로필 디렉터리입니다.
	Dir string
	// Close는 브라우저와 하위 프로세스를 정리합니다.
	Close func()
}

// ProfileDir는 프로필 이름에 대응하는 사용자 데이터 디렉터리를 반환합니다.
func ProfileDir(profile string) (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "chatctl", "profiles", profile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("프로필 디렉터리 생성 실패: %w", err)
	}
	return dir, nil
}

// Resolve는 옵션에 맞는 실제 엔진과 moli 실행 파일 경로를 고릅니다.
func Resolve(opts Options) (Engine, string) {
	if opts.Engine == EngineChrome {
		return EngineChrome, ""
	}
	// moli 는 헤드리스 전용이라 창이 필요하면 후보에서 빠집니다.
	if opts.Engine == EngineMoli || opts.Headless {
		if path, ok := MoliPath(); ok {
			return EngineMoli, path
		}
	}
	if opts.Engine == EngineMoli {
		return EngineMoli, ""
	}
	return EngineChrome, ""
}

// New는 브라우저 세션을 엽니다.
// moli 엔진이면 저장해 둔 로그인 쿠키까지 심어서 바로 쓸 수 있는 상태로 돌려줍니다.
func New(ctx context.Context, opts Options) (*Session, error) {
	dir, err := ProfileDir(opts.Profile)
	if err != nil {
		return nil, err
	}

	engine, moliBin := Resolve(opts)
	if engine == EngineMoli && !opts.Headless {
		return nil, fmt.Errorf("moli 엔진은 창을 띄울 수 없습니다. --engine chrome 을 사용하세요")
	}

	var (
		allocCtx    context.Context
		cancelAlloc context.CancelFunc
		stopEngine  = func() {}
	)

	switch engine {
	case EngineMoli:
		server, err := startMoli(ctx, moliBin, filepath.Join(dir, "moli"))
		if err != nil {
			return nil, err
		}
		stopEngine = server.stop
		allocCtx, cancelAlloc = chromedp.NewRemoteAllocator(ctx, server.url)
	default:
		allocCtx, cancelAlloc = chromedp.NewExecAllocator(ctx, append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.UserDataDir(dir),
			chromedp.Flag("headless", opts.Headless),
			chromedp.Flag("hide-scrollbars", opts.Headless),
			chromedp.Flag("mute-audio", opts.Headless),
		)...)
	}

	taskCtx, cancelTask := chromedp.NewContext(allocCtx)

	cancelTimeout := func() {}
	if opts.Timeout > 0 {
		taskCtx, cancelTimeout = context.WithTimeout(taskCtx, opts.Timeout)
	}

	sess := &Session{
		Ctx:    taskCtx,
		Engine: engine,
		Dir:    dir,
		Close: func() {
			cancelTimeout()
			cancelTask()
			cancelAlloc()
			stopEngine()
		},
	}

	// moli 는 chrome 프로필을 공유하지 못하므로 내보내 둔 쿠키로 로그인 상태를 복원합니다.
	if engine == EngineMoli {
		if err := session.Restore(taskCtx, dir); err != nil {
			sess.Close()
			return nil, err
		}
	}
	return sess, nil
}
