---
title: Codex와 OpenAI의 Agent Skills 확장
description: Agent Skills 공통 사양을 제외한 Codex의 탐색, 호출, 메타데이터, 지침 및 배포 구성
status: research
updated: 2026-08-31
---

# Codex와 OpenAI의 Agent Skills 확장

이 문서는 [Agent Skills 공통 사양](./skills.md)을 반복하지 않고 Codex와 OpenAI 생태계가 그 위에 추가하는 구성만 다룬다. 공식 OpenAI 문서가 현재 명시하는 범위와 `chat.md`의 선행 조사를 대조했다.

## 핵심 요약

Codex의 확장은 `SKILL.md` 자체를 실행 DSL로 키우기보다 주변 파일과 제품 구성으로 분리하는 형태다.

```text
portable skill
├── SKILL.md
├── scripts/
├── references/
├── assets/
└── agents/openai.yaml       # OpenAI 전용 UI·호출 정책·도구 의존성

host configuration
├── AGENTS.md                # 지속적인 프로젝트 지침
├── ~/.codex/config.toml     # Codex 설정과 skill 활성화 상태
└── .codex-plugin/plugin.json # 배포 패키지 manifest
```

## 발견과 호출

### 로컬 검색 위치

Codex는 시작한 현재 디렉터리부터 저장소 루트까지 올라가며 각 `.agents/skills`를 검색한다.

| 범위 | 위치 | 용도 |
| --- | --- | --- |
| 저장소 | `$CWD/.agents/skills`부터 `$REPO_ROOT/.agents/skills`까지 | 모듈·저장소별 공유 skill |
| 사용자 | `$HOME/.agents/skills` | 모든 저장소에서 쓰는 개인 skill |
| 관리자 | `/etc/codex/skills` | 머신·컨테이너 공용 skill |
| 시스템 | Codex에 번들 | OpenAI가 제공하는 기본 skill |

동일한 `name`의 skill을 합치지 않으며 둘 다 selector에 나타날 수 있다. skill 디렉터리 symlink도 따라간다. 파일 변경은 자동으로 감지하지만 반영되지 않으면 Codex를 다시 시작해야 한다.

### 호출 방식

- Codex CLI와 IDE extension: `/skills`에서 목록을 열거나 `$skill-name`으로 명시한다.
- ChatGPT: `@` selector로 명시한다.
- 암시적 호출: 사용자 요청과 `description`이 맞으면 호스트가 선택한다.

Codex의 최초 목록에는 `name`, `description` 외에 파일 경로도 들어간다. 이 목록은 모델 context window의 최대 2%, 크기를 알 수 없으면 8,000자로 제한된다. skill이 많으면 설명을 먼저 줄이고 일부 skill을 생략할 수도 있지만, 선택된 skill의 `SKILL.md`는 전체를 읽는다.

## `agents/openai.yaml`

OpenAI 전용 정보는 선택 파일인 `agents/openai.yaml`에 둔다.

```yaml
interface:
  display_name: "OpenAI Docs"
  short_description: "Search official OpenAI documentation"
  icon_small: "./assets/icon.svg"
  icon_large: "./assets/logo.png"
  brand_color: "#3B82F6"
  default_prompt: "Look this up in the official documentation"

policy:
  allow_implicit_invocation: false

dependencies:
  tools:
    - type: "mcp"
      value: "openaiDeveloperDocs"
      description: "OpenAI Docs MCP server"
      transport: "streamable_http"
      url: "https://developers.openai.com/mcp"
```

| 영역 | 필드 | 의미 |
| --- | --- | --- |
| `interface` | `display_name`, `short_description`, `icon_small`, `icon_large`, `brand_color`, `default_prompt` | ChatGPT desktop 등에서 보이는 표현과 시작 prompt |
| `policy` | `allow_implicit_invocation` | 기본값은 `true`. `false`면 자동 선택을 막고 명시적 `$skill` 호출은 유지한다. |
| `dependencies.tools` | `type`, `value`, `description`, `transport`, `url` | skill에 필요한 MCP 도구를 선언한다. |

표준 호환성을 위해 OpenAI 전용 메타데이터가 `SKILL.md` frontmatter가 아니라 별도 파일에 있다는 점이 중요하다.

## 활성화 상태 구성

로컬 skill을 삭제하지 않고 끄려면 `~/.codex/config.toml`에 정확한 `SKILL.md` 경로를 등록한다.

```toml
[[skills.config]]
path = "/path/to/skill/SKILL.md"
enabled = false
```

이 설정을 바꾼 뒤에는 Codex를 다시 시작한다. 현재 공개 문서에서 skill별 model, reasoning effort, subagent fork, hook을 `SKILL.md`에 선언하는 Codex 전용 frontmatter는 정의하지 않는다.

## `AGENTS.md`와 지속 지침

절차를 필요할 때만 로드하는 skill과 달리 `AGENTS.md`는 작업 전 읽는 지속 지침이다.

1. 전역 범위에서는 Codex home(기본 `~/.codex`)의 `AGENTS.override.md`를 먼저 찾고, 없으면 `AGENTS.md` 하나를 읽는다.
2. 프로젝트 범위에서는 저장소 루트부터 현재 디렉터리까지 내려오며 각 디렉터리에서 `AGENTS.override.md`, `AGENTS.md`, `project_doc_fallback_filenames` 순으로 하나를 선택한다.
3. 선택한 파일을 루트에서 현재 디렉터리 순으로 연결한다. 더 가까운 지침이 뒤에 놓여 앞선 지침을 덮는다.
4. 빈 파일은 건너뛰며 합산 크기는 `project_doc_max_bytes`의 제한을 받는다. 기본값은 32 KiB다.

| 목적 | 권장 위치 |
| --- | --- |
| 모든 프로젝트에 적용할 개인 지침 | `~/.codex/AGENTS.md` |
| 저장소 전체의 빌드·테스트·코딩 규칙 | 저장소 루트 `AGENTS.md` |
| 특정 하위 디렉터리의 예외 | 가까운 `AGENTS.override.md` |
| 반복 가능하지만 조건부인 긴 절차 | `.agents/skills/<name>/SKILL.md` |

## Plugin 배포 계층

로컬 디렉터리는 작성과 저장소 단위 공유에 적합하다. 여러 사용자·제품에 설치 가능한 단위로 배포하거나 connector와 함께 묶을 때는 plugin을 사용한다.

```text
my-plugin/
├── .codex-plugin/
│   └── plugin.json       # 필수 manifest
├── skills/               # 하나 이상의 skill
├── .app.json             # 등록된 MCP server connection 참조
├── .mcp.json             # plugin과 함께 배포하는 MCP server
└── assets/ 및 lifecycle hooks
```

Plugin은 skill, MCP 연결과 presentation asset을 하나의 패키지로 묶는다. 공개 plugin은 ChatGPT와 Codex가 공유하는 universal plugin directory에 게시된다. 이 계층은 Agent Skills 파일 표준이 아니라 OpenAI의 배포·통합 표준이다.

## Claude Code와의 구성 차이

| 항목 | Codex/OpenAI | Claude Code |
| --- | --- | --- |
| 명시적 skill 호출 | `$skill-name` (`/skills`는 selector) | `/skill-name` |
| 저장소 위치 | `.agents/skills/` | `.claude/skills/` |
| vendor 메타데이터 | `agents/openai.yaml`로 분리 | 대체로 `SKILL.md` frontmatter 확장 |
| 자동 호출 차단 | `allow_implicit_invocation: false` | `disable-model-invocation: true` |
| 개별 활성화 | `~/.codex/config.toml`의 `[[skills.config]]` | settings의 `skillOverrides` 또는 frontmatter |
| 지속 지침 | `AGENTS.md` / `AGENTS.override.md` | `CLAUDE.md` / `CLAUDE.local.md` / rules |
| 배포 manifest | `.codex-plugin/plugin.json` | `.claude-plugin/plugin.json` |
| skill 내 orchestration | 공개 Codex skill 문서에 별도 DSL 없음 | model·effort·fork·agent·hooks 등 지원 |

## 이식성 지침

- 공통 `SKILL.md`에는 Agent Skills 표준 필드만 둔다.
- OpenAI UI와 MCP 요구 사항은 `agents/openai.yaml`에 격리한다.
- `$skill-name`, `/skills`, `AGENTS.md`, `config.toml`은 Codex host 구성으로 문서화하고 portable 본문에서 필수 전제로 삼지 않는다.
- “문서에 없는 기능”과 “지원하지 않는 기능”을 동일시하지 않는다. 위 표의 orchestration 항목은 현재 공개 Codex skill 문서가 선언한 인터페이스 기준이다.

## 출처

- [OpenAI Docs — Build skills](https://learn.chatgpt.com/docs/build-skills)
- [OpenAI Docs — Custom instructions with AGENTS.md](https://learn.chatgpt.com/docs/agent-configuration/agents-md)
- [OpenAI Docs — Configuration reference](https://learn.chatgpt.com/docs/config-file/config-reference)
- [OpenAI Developers — Package your plugin](https://developers.openai.com/plugins/build/plugins)
- [Agent Skills — Specification](https://agentskills.io/specification)
