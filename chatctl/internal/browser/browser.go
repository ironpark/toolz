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
	"runtime"
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
	// UserDataDir가 지정되면 chatctl 관리 프로필 대신 기존 Chrome 사용자 데이터
	// 디렉터리를 그대로 씁니다. Chrome 은 프로필을 공유하지 못하는 moli 대신
	// 항상 chrome 엔진으로 실행됩니다.
	//
	// 비어 있으면 자동으로 정합니다: chatctl 프로필에 저장된 쿠키가 있으면 그
	// 프로필을, 없으면 OS 기본 위치에서 기존 Chrome 프로필을 탐색해 씁니다.
	UserDataDir string
	// NoAutoDetect가 true이면 UserDataDir 가 비어 있어도 기존 Chrome 프로필을
	// 탐색하지 않고 chatctl 관리 프로필만 씁니다. login 처럼 관리 프로필에
	// 세션을 만들어야 하는 명령이 사용합니다.
	NoAutoDetect bool
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
	// External은 chatctl 관리 프로필이 아닌 기존 Chrome 프로필을 쓰고 있다는 뜻입니다.
	External bool
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

// resolveDir는 세션이 쓸 사용자 데이터 디렉터리를 정합니다.
// 반환값 external 은 chatctl 관리 프로필이 아닌 기존 Chrome 프로필을 골랐다는 뜻입니다.
//
// 우선순위: 명시된 UserDataDir → 쿠키가 저장된 chatctl 프로필 → 자동 탐색된
// 기존 Chrome 프로필 → chatctl 프로필.
func resolveDir(opts Options) (dir string, external bool, err error) {
	if opts.UserDataDir != "" {
		dir, err = filepath.Abs(opts.UserDataDir)
		if err != nil {
			return "", false, err
		}
		info, err := os.Stat(dir)
		if err != nil {
			return "", false, fmt.Errorf("사용자 데이터 디렉터리를 열 수 없습니다: %w", err)
		}
		if !info.IsDir() {
			return "", false, fmt.Errorf("%s 은(는) 디렉터리가 아닙니다", dir)
		}
		return dir, true, nil
	}

	dir, err = ProfileDir(opts.Profile)
	if err != nil {
		return "", false, err
	}
	if opts.NoAutoDetect {
		return dir, false, nil
	}
	// chatctl 로 이미 로그인해 둔 프로필이 있으면 그대로 씁니다.
	if cookies, err := session.Load(dir); err == nil && len(cookies) > 0 {
		return dir, false, nil
	}
	if detected, ok := DetectChromeDir(); ok {
		return detected, true, nil
	}
	return dir, false, nil
}

// DetectChromeDir는 OS 기본 위치에서 기존 Chrome/Chromium 사용자 데이터
// 디렉터리를 찾습니다. 실제 프로필이 만들어진 흔적(Default/ 또는 Local State)이
// 있는 첫 번째 후보를 돌려줍니다.
func DetectChromeDir() (string, bool) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", false
	}
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		candidates = []string{
			filepath.Join(base, "Google", "Chrome"),
			filepath.Join(base, "Chromium"),
		}
	case "windows":
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			candidates = []string{
				filepath.Join(local, "Google", "Chrome", "User Data"),
				filepath.Join(local, "Chromium", "User Data"),
			}
		}
	default:
		candidates = []string{
			filepath.Join(base, "google-chrome"),
			filepath.Join(base, "chromium"),
		}
	}
	for _, dir := range candidates {
		if hasChromeProfile(dir) {
			return dir, true
		}
	}
	return "", false
}

// hasChromeProfile은 디렉터리가 실제 사용 중인 Chrome 프로필인지 확인합니다.
func hasChromeProfile(dir string) bool {
	if info, err := os.Stat(filepath.Join(dir, "Default")); err == nil && info.IsDir() {
		return true
	}
	if _, err := os.Stat(filepath.Join(dir, "Local State")); err == nil {
		return true
	}
	return false
}

// Resolve는 옵션에 맞는 실제 엔진과 moli 실행 파일 경로를 고릅니다.
func Resolve(opts Options) (Engine, string) {
	// 기존 Chrome 프로필은 moli 가 읽지 못하므로 chrome 으로 고정합니다.
	if opts.UserDataDir != "" {
		return EngineChrome, ""
	}
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
	dir, external, err := resolveDir(opts)
	if err != nil {
		return nil, err
	}
	if external {
		if opts.Engine == EngineMoli {
			return nil, fmt.Errorf("moli 엔진은 기존 Chrome 프로필을 쓸 수 없습니다. --engine chrome 을 사용하세요")
		}
		// 기존 Chrome 프로필은 moli 가 읽지 못하므로 chrome 으로 고정합니다.
		opts.Engine = EngineChrome
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
		Ctx:      taskCtx,
		Engine:   engine,
		Dir:      dir,
		External: external,
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
