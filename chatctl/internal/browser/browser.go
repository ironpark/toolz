// Package browser는 chromedp 세션을 열고 프로필 디렉터리를 관리합니다.
package browser

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
)

// Options는 브라우저 실행 방식을 정의합니다.
type Options struct {
	// Profile은 로그인 세션을 보관할 프로필 이름입니다.
	Profile string
	// Headless가 false이면 창을 띄웁니다. 최초 로그인 시 필요합니다.
	Headless bool
	// Timeout은 전체 작업 제한 시간입니다.
	Timeout time.Duration
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

// New는 chromedp 컨텍스트를 만들고 정리 함수를 반환합니다.
func New(ctx context.Context, opts Options) (context.Context, context.CancelFunc, error) {
	dir, err := ProfileDir(opts.Profile)
	if err != nil {
		return nil, nil, err
	}

	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(dir),
		chromedp.Flag("headless", opts.Headless),
		chromedp.Flag("hide-scrollbars", opts.Headless),
		chromedp.Flag("mute-audio", opts.Headless),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	taskCtx, cancelTask := chromedp.NewContext(allocCtx)

	cancelTimeout := func() {}
	if opts.Timeout > 0 {
		taskCtx, cancelTimeout = context.WithTimeout(taskCtx, opts.Timeout)
	}

	cancel := func() {
		cancelTimeout()
		cancelTask()
		cancelAlloc()
	}
	return taskCtx, cancel, nil
}
