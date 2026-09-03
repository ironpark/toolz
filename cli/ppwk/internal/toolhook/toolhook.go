// Package toolhook 은 코딩 에이전트 도구의 세션 훅 설정을 다룬다 (design §3.8 층 3).
//
// git 의 reference-transaction 훅과는 다른 것이다. 이쪽은 도구의 설정 파일에
// 등록되고 대화 세션의 시작·종료에 반응한다.
package toolhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Command 는 훅이 실행할 명령이다.
//
// 훅에서 실행되므로 빠르게 끝나야 하고, 알 수 없는 입력에는 조용히 exit 0
// 한다 (features §4).
const Command = "ppwk internal session-event"

// Events 는 등록하는 이벤트다.
//
// SubagentStart / SubagentStop 은 **일부러 빠져 있다.** 서브에이전트는 별도
// 세션이 아니라 부모 세션의 일부이며, 훅을 걸면 서브에이전트마다 유령
// 에이전트가 생긴다 (§3.8).
var Events = []string{"SessionStart", "SessionEnd"}

// ErrConflict 는 다른 ppwk 명령이 이미 등록돼 있다는 뜻이다.
var ErrConflict = errors.New("다른 ppwk 훅 설정이 이미 있습니다")

// Tool 은 지원하는 도구 하나다.
type Tool struct {
	// Name 은 --claude-code 같은 플래그 이름과 같다.
	Name string
	// Path 는 저장소 루트 기준 설정 파일 경로다.
	Path string
	// wrap 은 이벤트 하나에 붙일 항목을 만든다. 도구마다 모양이 다르다.
	wrap func(command string) any
	// commandsOf 는 이벤트 항목들에서 명령 문자열을 뽑는다.
	commandsOf func(entries []json.RawMessage) []string
}

// Tools 는 지원 도구 목록이다. 훅 표면이 대칭이라 같은 명령이 양쪽에 등록된다.
var Tools = []Tool{
	{
		Name: "claude-code",
		Path: filepath.Join(".claude", "settings.json"),
		// Claude Code 는 matcher 배열 안에 hooks 배열을 둔다.
		wrap: func(command string) any {
			return map[string]any{
				"hooks": []any{map[string]any{"type": "command", "command": command}},
			}
		},
		commandsOf: claudeCommands,
	},
	{
		Name: "codex",
		Path: filepath.Join(".codex", "hooks.json"),
		// Codex 는 평평한 목록이다.
		wrap: func(command string) any {
			return map[string]any{"type": "command", "command": command}
		},
		commandsOf: codexCommands,
	},
}

// ToolByName 은 이름으로 도구를 찾는다.
func ToolByName(name string) (Tool, bool) {
	for _, tool := range Tools {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// Status 는 한 도구의 설치 상태다.
type Status struct {
	Tool string `json:"tool"`
	Path string `json:"path"`
	// Configured 는 설정 파일이 있는지다. 우리 훅이 있다는 뜻은 아니다.
	Configured bool `json:"configured"`
	// Events 는 이벤트별 등록 여부다.
	Events map[string]bool `json:"events"`
	// Installed 는 모든 이벤트가 등록됐는지다. Configured 와 헷갈리기 쉬워
	// 사람이 보는 ✓/✗ 와 같은 판정을 JSON 에도 그대로 낸다.
	Installed bool `json:"installed"`
}

// allEventsRegistered 는 모든 이벤트가 등록됐는지다.
func (s Status) allEventsRegistered() bool {
	for _, event := range Events {
		if !s.Events[event] {
			return false
		}
	}
	return true
}

// Status 는 root 아래의 설정을 읽어 상태를 만든다.
func (t Tool) Status(root string) Status {
	status := Status{Tool: t.Name, Path: t.Path, Events: map[string]bool{}}
	config, err := t.load(root)
	if err != nil {
		return status
	}
	status.Configured = true
	for _, event := range Events {
		status.Events[event] = slices.Contains(t.commandsOf(config.hooks[event]), Command)
	}
	status.Installed = status.allEventsRegistered()
	return status
}

// Install 은 훅을 등록한다. 기존 설정은 보존하고 우리 항목만 더한다.
//
// 남의 훅과 나란히 두는 것은 병합이고, 우리 것으로 보이는데 내용이 다른 것이
// 충돌이다. 후자만 중단한다 — 도구 설정은 사람이 손대는 파일이라 임의로
// 덮어쓰면 안 된다.
func (t Tool) Install(root string, force bool) error {
	config, err := t.load(root)
	if err != nil && !os.IsNotExist(errors.Unwrap(err)) && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if config == nil {
		config = newConfig()
	}

	for _, event := range Events {
		entries := config.hooks[event]
		commands := t.commandsOf(entries)
		if slices.Contains(commands, Command) {
			continue
		}
		if stale := ppwkCommands(commands); len(stale) > 0 && !force {
			return fmt.Errorf("%s %s: %w (%v). --force 로 덮어쓸 수 있습니다",
				t.Name, event, ErrConflict, stale)
		}
		entries = dropPPWK(t, entries)

		added, err := json.Marshal(t.wrap(Command))
		if err != nil {
			return err
		}
		config.hooks[event] = append(entries, added)
	}
	return t.save(root, config)
}

// Uninstall 은 우리 항목만 지운다. 남의 훅과 다른 설정은 그대로 둔다.
func (t Tool) Uninstall(root string) error {
	config, err := t.load(root)
	if err != nil {
		// 설정이 없으면 지울 것도 없다. 멱등하다.
		return nil
	}
	for _, event := range Events {
		config.hooks[event] = dropPPWK(t, config.hooks[event])
		if len(config.hooks[event]) == 0 {
			delete(config.hooks, event)
		}
	}
	return t.save(root, config)
}

// config 는 도구 설정 파일이다.
//
// 아는 것은 hooks 뿐이고 나머지는 원본 바이트로 보존한다. 사람이 쓰는
// 파일이므로 우리가 모르는 설정을 지우면 안 된다 (§9.4 와 같은 이유).
type config struct {
	root  map[string]json.RawMessage
	hooks map[string][]json.RawMessage
}

func newConfig() *config {
	return &config{root: map[string]json.RawMessage{}, hooks: map[string][]json.RawMessage{}}
}

func (t Tool) load(root string) (*config, error) {
	data, err := os.ReadFile(filepath.Join(root, t.Path))
	if err != nil {
		return nil, err
	}
	c := newConfig()
	if err := json.Unmarshal(data, &c.root); err != nil {
		return nil, fmt.Errorf("%s 를 읽을 수 없습니다: %w", t.Path, err)
	}
	if raw, ok := c.root["hooks"]; ok {
		if err := json.Unmarshal(raw, &c.hooks); err != nil {
			return nil, fmt.Errorf("%s 의 hooks 를 읽을 수 없습니다: %w", t.Path, err)
		}
	}
	return c, nil
}

func (t Tool) save(root string, c *config) error {
	path := filepath.Join(root, t.Path)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	hooks, err := json.Marshal(c.hooks)
	if err != nil {
		return err
	}
	c.root["hooks"] = hooks

	data, err := json.MarshalIndent(c.root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// dropPPWK 는 ppwk 가 등록한 항목만 걷어낸다.
func dropPPWK(t Tool, entries []json.RawMessage) []json.RawMessage {
	kept := make([]json.RawMessage, 0, len(entries))
	for _, entry := range entries {
		if len(ppwkCommands(t.commandsOf([]json.RawMessage{entry}))) > 0 {
			continue
		}
		kept = append(kept, entry)
	}
	return kept
}

// ppwkCommands 는 ppwk 가 등록한 것으로 보이는 명령만 고른다.
func ppwkCommands(commands []string) []string {
	var out []string
	for _, command := range commands {
		if strings.Contains(command, "ppwk") && strings.Contains(command, "session-event") {
			out = append(out, command)
		}
	}
	return out
}

// claudeCommands 는 {"hooks":[{"command":...}]} 모양에서 명령을 뽑는다.
func claudeCommands(entries []json.RawMessage) []string {
	var out []string
	for _, entry := range entries {
		var matcher struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		}
		if json.Unmarshal(entry, &matcher) != nil {
			continue
		}
		for _, hook := range matcher.Hooks {
			out = append(out, hook.Command)
		}
	}
	return out
}

// codexCommands 는 {"command":...} 모양에서 명령을 뽑는다.
func codexCommands(entries []json.RawMessage) []string {
	var out []string
	for _, entry := range entries {
		var hook struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(entry, &hook) != nil {
			continue
		}
		if hook.Command != "" {
			out = append(out, hook.Command)
		}
	}
	return out
}
