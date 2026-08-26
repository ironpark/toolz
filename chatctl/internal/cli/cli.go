// Package cli는 chatctl 명령을 정의합니다.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/urfave/cli/v3"

	"github.com/ironpark/toolz/chatctl/internal/browser"
	"github.com/ironpark/toolz/chatctl/internal/provider"
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

			bctx, cancel, err := browser.New(ctx, browser.Options{
				Profile:  cmd.String("profile"),
				Headless: false,
				Timeout:  cmd.Duration("timeout"),
			})
			if err != nil {
				return err
			}
			defer cancel()

			if err := chromedp.Run(bctx, chromedp.Navigate(p.HomeURL)); err != nil {
				return fmt.Errorf("브라우저를 열지 못했습니다: %w", err)
			}
			fmt.Printf("%s 로그인 창을 열었습니다. 로그인을 마친 뒤 Enter 를 누르세요.\n", p.Name)
			fmt.Scanln()
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
			&cli.BoolFlag{Name: "show", Usage: "브라우저 창을 표시합니다 (디버깅용)"},
			&cli.IntFlag{Name: "limit", Usage: "서비스별 최대 출력 개수 (0이면 전체)"},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			targets, err := resolveTargets(cmd.Args().Slice())
			if err != nil {
				return err
			}

			timeout := cmd.Duration("timeout")
			bctx, cancel, err := browser.New(ctx, browser.Options{
				Profile:  cmd.String("profile"),
				Headless: !cmd.Bool("show"),
				Timeout:  timeout,
			})
			if err != nil {
				return err
			}
			defer cancel()

			var all []provider.Conversation
			var failures []error
			for _, p := range targets {
				convs, err := p.List(bctx, timeout/time.Duration(len(targets)))
				if err != nil {
					failures = append(failures, err)
					continue
				}
				if limit := int(cmd.Int("limit")); limit > 0 && len(convs) > limit {
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
			target := cmd.Args().Get(1)
			if target == "" {
				return fmt.Errorf("열 대화 ID 또는 URL 을 지정하세요")
			}
			if !strings.HasPrefix(target, "http") {
				target = strings.TrimSuffix(p.HomeURL, "/") + "/" + strings.TrimPrefix(target, "/")
			}

			bctx, cancel, err := browser.New(ctx, browser.Options{
				Profile:  cmd.String("profile"),
				Headless: false,
				Timeout:  cmd.Duration("timeout"),
			})
			if err != nil {
				return err
			}
			defer cancel()

			if err := chromedp.Run(bctx, chromedp.Navigate(target)); err != nil {
				return fmt.Errorf("대화를 열지 못했습니다: %w", err)
			}
			fmt.Printf("%s 을(를) 열었습니다. 창을 닫으려면 Enter 를 누르세요.\n", target)
			fmt.Scanln()
			return nil
		},
	}
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
