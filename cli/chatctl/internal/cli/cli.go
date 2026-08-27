// Package cli는 chatctl 명령을 정의합니다.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/urfave/cli/v3"

	"github.com/ironpark/toolz/cli/chatctl/internal/browser"
	"github.com/ironpark/toolz/cli/chatctl/internal/provider"
	"github.com/ironpark/toolz/cli/chatctl/internal/session"
)

// New는 루트 명령을 만듭니다.
func New(version string) *cli.Command {
	return &cli.Command{
		Name:    "chatctl",
		Usage:   "ChatGPT, Gemini, Claude 웹 세션의 대화 목록을 관리합니다",
		Version: version,
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "profile",
				Aliases: []string{"p"},
				Usage:   "브라우저 프로필 이름 (로그인 세션 보관 단위)",
				Value:   "default",
				Sources: cli.EnvVars("CHATCTL_PROFILE"),
			},
			&cli.StringFlag{
				Name:    "engine",
				Aliases: []string{"e"},
				Usage:   "브라우저 엔진 (auto|moli|chrome). auto 는 moli 가 설치되어 있으면 moli 를 씁니다",
				Value:   string(browser.EngineAuto),
				Sources: cli.EnvVars("CHATCTL_ENGINE"),
				Validator: func(s string) error {
					_, err := browser.ParseEngine(s)
					return err
				},
			},
			&cli.StringFlag{
				Name:    "user-data-dir",
				Aliases: []string{"d"},
				Usage:   "기존 Chrome 사용자 데이터 디렉터리를 그대로 사용합니다 (chrome 엔진으로 전환됩니다). 해당 프로필을 쓰는 Chrome 은 먼저 종료해야 합니다",
				Sources: cli.EnvVars("CHATCTL_USER_DATA_DIR"),
			},
			&cli.DurationFlag{
				Name:  "timeout",
				Usage: "브라우저 작업 제한 시간",
				Value: 90 * time.Second,
			},
		},
		Commands: []*cli.Command{
			loginCommand(),
			listCommand(),
			openCommand(),
			providersCommand(),
			doctorCommand(),
		},
	}
}

func providersCommand() *cli.Command {
	return &cli.Command{
		Name:  "providers",
		Usage: "지원하는 서비스 목록을 출력합니다",
		Action: func(_ context.Context, _ *cli.Command) error {
			for _, p := range provider.All {
				fmt.Printf("%-8s %s\n", p.Name, p.HomeURL)
			}
			return nil
		},
	}
}

func doctorCommand() *cli.Command {
	return &cli.Command{
		Name:  "doctor",
		Usage: "엔진과 프로필 상태를 점검합니다",
		Action: func(_ context.Context, cmd *cli.Command) error {
			if path, ok := browser.MoliPath(); ok {
				fmt.Printf("moli:    %s\n", path)
			} else {
				fmt.Println("moli:    미설치 (https://github.com/lexmount/moli) — chrome 으로 동작합니다")
			}

			if dir := cmd.String("user-data-dir"); dir != "" {
				if info, err := os.Stat(dir); err != nil || !info.IsDir() {
					return fmt.Errorf("사용자 데이터 디렉터리를 열 수 없습니다: %s", dir)
				}
				fmt.Printf("프로필:  기존 Chrome 사용자 데이터 디렉터리 (%s)\n", dir)
				fmt.Println("쿠키:    Chrome 프로필의 로그인 세션을 그대로 사용합니다")
				return nil
			}

			profile := cmd.String("profile")
			dir, err := browser.ProfileDir(profile)
			if err != nil {
				return err
			}
			fmt.Printf("프로필:  %s (%s)\n", profile, dir)

			cookies, err := session.Load(dir)
			if err != nil {
				return err
			}
			if len(cookies) == 0 {
				if detected, ok := browser.DetectChromeDir(); ok {
					fmt.Printf("쿠키:    저장된 쿠키 없음 — 자동 탐색된 기존 Chrome 프로필을 사용합니다 (%s)\n", detected)
				} else {
					fmt.Println("쿠키:    유효한 쿠키 없음 — `chatctl login <서비스>` 로 로그인하세요")
				}
			} else {
				fmt.Printf("쿠키:    유효 %d개 (%s)\n", len(cookies), session.CookiePath(dir))
			}
			return nil
		},
	}
}

func loginCommand() *cli.Command {
	return &cli.Command{
		Name:      "login",
		Usage:     "브라우저 창을 열어 서비스에 로그인합니다 (세션은 프로필에 저장됩니다)",
		ArgsUsage: "<chatgpt|gemini|claude>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			p, err := provider.Get(cmd.Args().First())
			if err != nil {
				return err
			}

			// 로그인은 사람이 직접 입력해야 하므로 창이 필요합니다. 엔진 선택은 browser 가 맡습니다.
			// -d 를 명시하지 않았다면 관리 프로필에 세션을 만들어야 하므로 자동 탐색을 끕니다.
			sess, err := openSession(ctx, cmd, browser.Options{Headless: false, NoAutoDetect: true})
			if err != nil {
				return err
			}
			defer sess.Close()

			if err := chromedp.Run(sess.Ctx, chromedp.Navigate(p.HomeURL)); err != nil {
				return fmt.Errorf("브라우저를 열지 못했습니다: %w", err)
			}
			fmt.Printf("%s 로그인 창을 열었습니다. 로그인을 마친 뒤 Enter 를 누르세요.\n", p.Name)
			fmt.Scanln()

			// 기존 Chrome 프로필은 로그인 세션을 스스로 보관하므로 내보낼 필요가 없습니다.
			if sess.External {
				fmt.Println("로그인 세션이 Chrome 프로필에 저장되었습니다.")
				return nil
			}

			// moli 는 chrome 프로필을 공유하지 못하므로 쿠키를 따로 내보내 둡니다.
			n, err := session.Save(sess.Ctx, sess.Dir)
			if err != nil {
				return err
			}
			fmt.Printf("쿠키 %d개를 저장했습니다: %s\n", n, session.CookiePath(sess.Dir))
			return nil
		},
	}
}

func listCommand() *cli.Command {
	return &cli.Command{
		Name:      "list",
		Usage:     "대화 목록을 조회합니다 (서비스를 생략하면 전체)",
		ArgsUsage: "[chatgpt|gemini|claude ...]",
		Flags: []cli.Flag{
			&cli.BoolFlag{Name: "json", Usage: "JSON 으로 출력합니다"},
			&cli.BoolFlag{Name: "show", Usage: "브라우저 창을 표시합니다 (chrome 엔진으로 전환됩니다)"},
			&cli.IntFlag{Name: "limit", Usage: "서비스별 최대 개수 (0이면 끝까지 스크롤해 전체를 가져옵니다)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			targets, err := resolveTargets(cmd.Args().Slice())
			if err != nil {
				return err
			}

			sess, err := openSession(ctx, cmd, browser.Options{Headless: !cmd.Bool("show")})
			if err != nil {
				return err
			}
			defer sess.Close()

			timeout := cmd.Duration("timeout")
			limit := int(cmd.Int("limit"))
			var all []provider.Conversation
			var failures []error
			for _, p := range targets {
				convs, err := p.List(sess.Ctx, limit, timeout/time.Duration(len(targets)))
				if err != nil {
					failures = append(failures, err)
					continue
				}
				if limit > 0 && len(convs) > limit {
					convs = convs[:limit]
				}
				all = append(all, convs...)
			}

			if err := render(all, cmd.Bool("json")); err != nil {
				return err
			}
			for _, err := range failures {
				fmt.Fprintf(os.Stderr, "경고: %v\n", err)
			}
			if len(all) == 0 && len(failures) > 0 {
				return fmt.Errorf("대화 목록을 가져오지 못했습니다. `chatctl login <서비스>` 로 먼저 로그인하세요")
			}
			return nil
		},
	}
}

func openCommand() *cli.Command {
	return &cli.Command{
		Name:      "open",
		Usage:     "대화 ID 또는 URL 을 브라우저 창으로 엽니다",
		ArgsUsage: "<chatgpt|gemini|claude> <대화ID|URL>",
		Action: func(ctx context.Context, cmd *cli.Command) error {
			p, err := provider.Get(cmd.Args().First())
			if err != nil {
				return err
			}
			if cmd.Args().Get(1) == "" {
				return fmt.Errorf("열 대화 ID 또는 URL 을 지정하세요")
			}
			target := p.ConversationURL(cmd.Args().Get(1))

			// 사람이 보라고 여는 명령이므로 창이 필요합니다.
			sess, err := openSession(ctx, cmd, browser.Options{Headless: false})
			if err != nil {
				return err
			}
			defer sess.Close()

			if err := chromedp.Run(sess.Ctx, chromedp.Navigate(target)); err != nil {
				return fmt.Errorf("대화를 열지 못했습니다: %w", err)
			}
			fmt.Printf("%s 을(를) 열었습니다. 창을 닫으려면 Enter 를 누르세요.\n", target)
			fmt.Scanln()
			return nil
		},
	}
}

// openSession은 전역 플래그를 반영해 브라우저 세션을 엽니다.
// 호출하는 쪽은 창이 필요한지(Headless)만 정합니다.
func openSession(ctx context.Context, cmd *cli.Command, opts browser.Options) (*browser.Session, error) {
	engine, err := browser.ParseEngine(cmd.String("engine"))
	if err != nil {
		return nil, err
	}
	opts.Engine = engine
	opts.Profile = cmd.String("profile")
	opts.UserDataDir = cmd.String("user-data-dir")
	opts.Timeout = cmd.Duration("timeout")
	return browser.New(ctx, opts)
}

func resolveTargets(args []string) ([]*provider.Provider, error) {
	if len(args) == 0 {
		return provider.All, nil
	}
	targets := make([]*provider.Provider, 0, len(args))
	for _, name := range args {
		p, err := provider.Get(name)
		if err != nil {
			return nil, err
		}
		targets = append(targets, p)
	}
	return targets, nil
}

func render(convs []provider.Conversation, asJSON bool) error {
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if convs == nil {
			convs = []provider.Conversation{}
		}
		return enc.Encode(convs)
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "SERVICE\tID\tTITLE")
	for _, c := range convs {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Provider, c.ID, c.Title)
	}
	return w.Flush()
}
