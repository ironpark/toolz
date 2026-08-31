---
title: Claude Code의 Agent Skills 확장
description: Agent Skills 공통 사양을 제외한 Claude Code의 frontmatter, 실행, 탐색, 지침 및 plugin 구성
status: research
updated: 2026-08-31
---

# Claude Code의 Agent Skills 확장

이 문서는 [Agent Skills 공통 사양](./skills.md)을 반복하지 않고 Claude Code가 추가하거나 다르게 해석하는 구성만 다룬다. 공식 Claude Code 문서와 `chat.md`의 선행 조사를 대조했다.

## 핵심 요약

Claude Code는 `SKILL.md`를 재사용 가능한 지침뿐 아니라 호출 정책과 실행 orchestration을 선언하는 단위로 확장한다.

```text
SKILL.md
├── invocation control
├── arguments and substitutions
├── tool grants and restrictions
├── model and effort override
├── subagent fork and background execution
├── hooks and path activation
└── dynamic shell context injection
```

이 필드와 본문 문법은 로컬 Claude Code 확장이다. claude.ai skill upload, Skills API, `package_skill.py` 배포 경로는 표준 여섯 필드만 허용하며 알 수 없는 키를 무시하지 않고 오류로 처리한다.

## 발견과 호출

### 저장 위치와 우선순위

| 범위 | 위치 | 충돌 규칙 |
| --- | --- | --- |
| Enterprise | managed settings의 `.claude/skills/` | 가장 높은 우선순위 |
| Personal | `~/.claude/skills/<name>/SKILL.md` | project보다 우선 |
| Project | `.claude/skills/<name>/SKILL.md` | 저장소·디렉터리별 공유 |
| Plugin | `<plugin>/skills/<name>/SKILL.md` | `/plugin-name:skill-name` namespace 사용 |

로컬 충돌은 enterprise → personal → project 순으로 해결한다. plugin은 namespace로 충돌을 피한다. legacy `.claude/commands/<name>.md`도 계속 동작하지만 같은 이름의 skill이 우선하며, 새 구성에는 skill이 권장된다.

프로젝트 skill은 시작 디렉터리부터 저장소 루트까지의 `.claude/skills/`에서 발견된다. 하위 디렉터리의 skill은 Claude가 그 디렉터리의 파일을 읽거나 편집할 때 추가되며, 이름이 겹치면 디렉터리 한정 이름으로 함께 유지된다. `--add-dir`로 추가한 디렉터리의 skills와 commands도 예외적으로 자동 탐색한다.

Claude Code는 개인·프로젝트 skill의 `SKILL.md` 변경을 실행 중 감지한다. skill 디렉터리 symlink도 지원하며 같은 대상은 한 번만 로드한다.

### 호출 방식

- 사용자는 `/skill-name`으로 명시한다.
- 기본적으로 Claude도 `description`과 `when_to_use`를 보고 자동 호출할 수 있다.
- `/skills`는 설치된 skill을 확인하는 메뉴다.
- plugin skill은 `/plugin-name:skill-name`으로 namespace된다.

개인·프로젝트 skill의 실제 command 이름은 frontmatter `name`이 아니라 디렉터리 이름에서 온다. `name`은 표시 이름이다. plugin에서는 `name`이 command 마지막 segment를 바꿀 수 있다.

## 표준과 다른 frontmatter 해석

Claude Code 로컬 런타임은 모든 frontmatter 필드를 선택 사항으로 취급한다. `name`이 없으면 디렉터리 이름, `description`이 없으면 Markdown 첫 문단을 사용한다. 이는 `name`과 `description`을 필수로 요구하는 Agent Skills 표준보다 관대하다.

이 편의 동작에 의존한 파일은 표준 validator나 다른 호스트에서 실패할 수 있다. 이식 가능한 skill은 Claude Code에서도 두 필드를 모두 명시해야 한다.

## Claude Code 전용 frontmatter

| 필드 | 의미와 실행 범위 |
| --- | --- |
| `when_to_use` | 자동 호출 조건을 추가한다. `description`과 합쳐 listing에 들어가며 합산 1,536자에서 잘릴 수 있다. |
| `argument-hint` | autocomplete에 인자 형식을 표시한다. |
| `arguments` | 공백 문자열 또는 YAML 목록으로 이름 있는 위치 인자를 선언한다. |
| `disable-model-invocation` | `true`면 Claude의 자동 호출을 막고 사용자 직접 호출만 허용한다. 설명도 Claude context에서 제외한다. |
| `user-invocable` | `false`면 `/` 메뉴와 직접 호출을 막고 Claude만 사용할 수 있게 한다. |
| `allowed-tools` | 호출한 turn 동안 도구를 사전 승인한다. 다음 사용자 메시지에서 grant가 해제되며 다른 도구를 제한하지는 않는다. |
| `disallowed-tools` | 호출한 turn 동안 지정한 도구를 사용 가능 목록에서 제거한다. |
| `model` | 현재 turn 또는 fork된 subagent의 model을 덮어쓴다. 다음 사용자 prompt에서는 session model로 돌아간다. |
| `effort` | 현재 skill의 reasoning effort를 덮어쓴다. 지원 값은 model에 따라 달라진다. |
| `context` | `fork`이면 별도 subagent context에서 실행한다. |
| `agent` | `context: fork`일 때 사용할 built-in 또는 custom subagent를 지정한다. |
| `background` | fork된 skill을 background로 실행할지 정한다. 기본값은 `true`이며 `false`면 결과를 기다린다. |
| `hooks` | 호출 시 hook을 등록하고 나머지 session 동안 유지한다. |
| `paths` | glob과 일치하는 파일을 작업할 때만 자동 활성화한다. |
| `shell` | 동적 context 명령에 `bash` 또는 `powershell`을 지정한다. |

`license`, `compatibility`, `metadata`, `allowed-tools`는 표준에도 존재한다. 다만 Claude Code는 `license`와 `compatibility`를 받아들이되 자체 동작에는 사용하지 않고, `allowed-tools`에는 turn 단위 권한 부여라는 구체적 의미를 부여한다.

### 호출 제어 조합

| 구성 | 사용자 호출 | Claude 자동 호출 |
| --- | --- | --- |
| 기본값 | 가능 | 가능 |
| `disable-model-invocation: true` | 가능 | 불가 |
| `user-invocable: false` | 불가 | 가능 |

부작용이 큰 deploy·commit 절차는 manual-only, 직접 실행할 의미가 없는 배경 지식은 model-only에 적합하다.

## 인자와 문자열 치환

skill 호출 뒤의 문자열은 `$ARGUMENTS`로 받는다. 위치 인자는 `$ARGUMENTS[0]` 또는 `$0` 형식을 사용하며, `arguments`에 이름을 선언하면 이름으로도 치환할 수 있다.

```yaml
---
name: migrate-component
description: Migrate a component between languages.
arguments:
  - component
  - from
  - to
---

Migrate $component from $from to $to.
```

인자를 전달했지만 본문에 치환 placeholder가 없으면 Claude Code가 본문 끝에 `ARGUMENTS: ...`를 추가한다. 여러 inline skill을 한 메시지 시작 부분에 쌓는 기능도 있으나 개수와 fork 동작에는 버전별 제한이 있으므로 portable workflow의 전제로 삼지 않는 편이 안전하다.

## 동적 context injection

본문의 ``!`command` ``는 skill 내용을 Claude에게 보내기 전에 shell 명령을 실행하고 그 출력을 해당 위치에 넣는다.

```markdown
## Pull request context

- Diff: !`gh pr diff`
- Files: !`gh pr diff --name-only`
```

여러 줄 명령은 `!`가 붙은 fenced code block으로 쓸 수 있다. 명령은 원본 파일에 대해 한 차례만 치환되며 출력은 다시 해석하지 않는다. 권한 검사가 허용 이외의 결과를 내거나 명령이 실패하면 전체 skill 호출을 중단하고 Claude는 렌더링된 본문을 받지 못한다.

이 기능은 로컬 Claude Code의 실행 semantics다. claude.ai에서 동기화된 skill은 로컬 머신에서 이 명령을 실행하지 않으며 API·chat 경로에서도 같은 동작을 기대할 수 없다. 관리자는 `disableSkillShellExecution`으로 사용자·프로젝트·plugin skill의 실행을 막을 수 있다.

## Subagent 실행

```yaml
---
name: deep-research
description: Research a codebase topic thoroughly.
context: fork
agent: Explore
background: false
model: opus
effort: high
---

Research $ARGUMENTS and return findings with file references.
```

`context: fork`이면 skill 본문이 별도 subagent의 task prompt가 되고 기존 대화 기록은 전달되지 않는다. `agent`를 생략하면 `general-purpose`를 사용한다. 기본적으로 background 실행하며 `background: false`면 호출 turn에서 결과를 기다린다.

Explore와 Plan agent는 작은 context를 위해 `CLAUDE.md`와 git status를 생략한다. 별도 task가 없는 단순 참고 지침은 fork된 agent가 수행할 작업이 없으므로 inline skill에 더 적합하다.

## 내용 수명과 권한 수명

호출해 렌더링된 `SKILL.md`는 하나의 메시지로 대화에 들어가 이후 turn에도 남는다. 반면 `allowed-tools`와 `disallowed-tools`의 효과는 다음 사용자 메시지에서 끝난다.

compaction 후에는 skill별 최신 호출의 앞 5,000 tokens를 다시 붙이며 전체 재첨부 예산은 25,000 tokens다. 최근 skill부터 채우므로 오래된 skill은 빠질 수 있다. 이 수치는 Agent Skills 표준이 아니라 Claude Code의 현재 runtime 정책이다.

## `CLAUDE.md`, settings, plugin

### 지속 지침

`CLAUDE.md`는 session 시작 때 로드되는 지속 지침이고 skill은 필요할 때 로드되는 절차다.

| 범위 | 위치 |
| --- | --- |
| Managed | OS별 managed policy 경로의 `CLAUDE.md` |
| User | `~/.claude/CLAUDE.md` |
| Project | `./CLAUDE.md` 또는 `./.claude/CLAUDE.md` |
| Local | `./CLAUDE.local.md` |

현재 작업 디렉터리 위 계층의 파일은 시작할 때 전체 로드하고, 하위 디렉터리의 파일은 해당 위치를 작업할 때 읽는다. 대형 저장소는 `.claude/rules/`로 주제·경로별 규칙을 나눌 수 있다.

### settings

JSON 설정 우선순위는 managed settings → command-line → `.claude/settings.local.json` → `.claude/settings.json` → `~/.claude/settings.json` 순이다. 권한은 deny → ask → allow 순으로 평가되며 상위 범위의 deny를 하위 범위에서 허용할 수 없다.

### plugin

```text
my-plugin/
├── .claude-plugin/
│   └── plugin.json
├── skills/
├── agents/
├── hooks/hooks.json
├── .mcp.json
├── .lsp.json
├── monitors/monitors.json
├── bin/
└── settings.json
```

Claude plugin은 skill뿐 아니라 custom agent, hook, MCP/LSP server, monitor와 executable을 함께 묶을 수 있다. `.claude-plugin/` 안에는 manifest만 두고 구성 요소 디렉터리는 plugin 루트에 둔다. plugin 루트의 `CLAUDE.md`는 project context로 로드되지 않으므로 배포할 지침은 skill이나 agent에 넣는다.

## 이식성 지침

- 공통 core는 표준의 `name`, `description`을 포함하고 표준 여섯 필드만 사용한다.
- Claude 전용 frontmatter와 본문 치환 문법이 필요하면 로컬 Claude Code 전용 wrapper skill로 분리한다.
- claude.ai 업로드와 Skills API에도 배포할 skill에는 Claude 전용 키를 넣지 않는다. validator가 알 수 없는 키를 hard error로 거부한다.
- 동적 shell injection과 `allowed-tools`는 저장소에서 코드를 실행할 권한을 넓힐 수 있으므로 외부 skill 설치 전에 검토한다.
- `/skill-name`, `CLAUDE.md`, `.claude/settings.json`은 host 구성으로 취급하고 portable 본문에서 필수 전제로 삼지 않는다.

## 출처

- [Claude Code Docs — Extend Claude with skills](https://code.claude.com/docs/en/skills)
- [Claude Code Docs — How Claude remembers your project](https://code.claude.com/docs/en/memory)
- [Claude Code Docs — Claude Code settings](https://code.claude.com/docs/en/settings)
- [Claude Code Docs — Configure permissions](https://code.claude.com/docs/en/permissions)
- [Claude Code Docs — Create plugins](https://code.claude.com/docs/en/plugins)
- [Claude Code Docs — Plugins reference](https://code.claude.com/docs/en/plugins-reference)
- [Agent Skills — Specification](https://agentskills.io/specification)
