// Package session은 로그인 쿠키를 프로필에 저장하고 다시 주입합니다.
//
// chrome 엔진은 자체 프로필 디렉터리에 세션을 유지하지만, moli 는 별도 프로세스라
// 로그인 결과를 공유하지 못합니다. 그래서 `login` 이 끝날 때 쿠키를 파일로 내보내고
// moli 세션 시작 시 CDP 로 다시 심어 줍니다.
package session

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/storage"
	"github.com/chromedp/chromedp"
)

// CookiePath는 프로필 디렉터리 안의 쿠키 파일 경로입니다.
func CookiePath(profileDir string) string {
	return filepath.Join(profileDir, "cookies.json")
}

// Save는 현재 브라우저의 쿠키를 프로필에 기록합니다.
// 주입할 때 그대로 쓸 수 있도록 CDP 파라미터 형태로 저장합니다.
func Save(ctx context.Context, profileDir string) (int, error) {
	var saved []*network.CookieParam
	err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		cookies, err := storage.GetCookies().Do(ctx)
		if err != nil {
			return err
		}
		saved = make([]*network.CookieParam, 0, len(cookies))
		for _, c := range cookies {
			param := &network.CookieParam{
				Name:     c.Name,
				Value:    c.Value,
				Domain:   c.Domain,
				Path:     c.Path,
				Secure:   c.Secure,
				HTTPOnly: c.HTTPOnly,
				SameSite: c.SameSite,
			}
			if c.Expires > 0 {
				exp := cdp.TimeSinceEpoch(time.Unix(int64(c.Expires), 0))
				param.Expires = &exp
			}
			saved = append(saved, param)
		}
		return nil
	}))
	if err != nil {
		return 0, fmt.Errorf("쿠키를 읽지 못했습니다: %w", err)
	}

	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return 0, err
	}
	// 세션 쿠키가 담기므로 소유자만 읽도록 둡니다.
	if err := os.WriteFile(CookiePath(profileDir), data, 0o600); err != nil {
		return 0, fmt.Errorf("쿠키 저장 실패: %w", err)
	}
	return len(saved), nil
}

// Load는 저장된 쿠키 중 아직 만료되지 않은 것만 읽습니다.
// 파일이 없으면 빈 목록을 돌려줍니다.
func Load(profileDir string) ([]*network.CookieParam, error) {
	data, err := os.ReadFile(CookiePath(profileDir))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("쿠키를 읽지 못했습니다: %w", err)
	}
	var cookies []*network.CookieParam
	if err := json.Unmarshal(data, &cookies); err != nil {
		return nil, fmt.Errorf("쿠키 파일 파싱 실패: %w", err)
	}

	now := time.Now()
	live := cookies[:0]
	for _, c := range cookies {
		if c.Expires != nil && time.Time(*c.Expires).Before(now) {
			continue
		}
		live = append(live, c)
	}
	return live, nil
}

// Restore는 저장된 쿠키를 현재 브라우저에 한 번에 주입합니다.
func Restore(ctx context.Context, profileDir string) error {
	cookies, err := Load(profileDir)
	if err != nil {
		return err
	}
	if len(cookies) == 0 {
		return nil
	}

	err = chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
		return network.SetCookies(cookies).Do(ctx)
	}))
	if err != nil {
		return fmt.Errorf("쿠키 주입 실패: %w", err)
	}
	return nil
}
