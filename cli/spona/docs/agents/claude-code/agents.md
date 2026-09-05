# Claude Code (`claude`) — 서브에이전트 정의 포맷

> [README](README.md) · [../](../README.md)

출처: <https://code.claude.com/docs/en/sub-agents>

서브에이전트는 YAML frontmatter + Markdown 시스템 프롬프트로 정의합니다.
같은 이름이 여러 곳에 있으면 우선순위가 높은 쪽이 이깁니다.

| 위치 | 스코프 | 우선순위 |
| --- | --- | --- |
| managed settings | 조직 전체 | 1 (최상) |
| `--agents` CLI 플래그 | 현재 세션 | 2 |
| `<project>/.claude/agents/` | 프로젝트 | 3 |
| `~/.claude/agents/` | 모든 프로젝트 | 4 |
| 플러그인의 `agents/` | 플러그인 활성 범위 | 5 (최하) |

두 디렉터리 모두 **재귀 탐색**되므로 하위 폴더로 정리할 수 있습니다.

```markdown
---
name: code-reviewer
description: Reviews code for quality and best practices
tools: Read, Glob, Grep
model: sonnet
---

You are a code reviewer. ...
```

필수 필드: `name`(소문자·하이픈, 콜론이나 선행 하이픈 불가), `description`.

선택 필드: `tools`, `disallowedTools`, `model`(별칭·전체 ID·`inherit`), `permissionMode`,
`maxTurns`, `skills`, `mcpServers`, `hooks`, `memory`(`user`/`project`/`local`),
`background`, `effort`, `isolation`(`worktree`), `color`, `initialPrompt`, `experimental`.

`--agents` JSON은 위 필드 전부에 더해 `prompt`(마크다운 본문에 해당)를 받습니다.
최상위 키가 에이전트 이름입니다.

```sh
claude --agents '{"code-reviewer":{"description":"...","prompt":"...","tools":["Read","Grep"],"model":"sonnet"}}'
```

모델 해석 순서: 호출별 `model` 파라미터 → 정의의 `model`(`inherit`이면 메인 대화 모델) →
`CLAUDE_CODE_SUBAGENT_MODEL` → 메인 대화 모델.

> [!WARNING]
> `name`이 없거나, 여는 `---`가 첫 줄이 아니거나, `description`이 없거나, YAML 파싱에
> 실패한 파일은 **보고 없이 조용히 무시**됩니다. spona가 에이전트 파일을 생성한다면
> `claude plugin validate .claude/agents/` 로 검증하는 단계를 넣는 것이 좋습니다.
