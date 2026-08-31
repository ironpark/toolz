---
title: Agent Skills 표준 사양
description: agentskills.io가 정의하는 이식 가능한 SKILL.md 형식과 구성 규칙
status: research
updated: 2026-08-31
---

# Agent Skills 표준 사양

이 문서는 Claude Code나 Codex에 종속되지 않는 [Agent Skills 공개 사양](https://agentskills.io/specification)의 공통 부분만 정리한다. 제품별 탐색 경로, 호출 문법, 추가 frontmatter와 패키징 방식은 [Claude Code 확장](./claude.md)과 [Codex/OpenAI 확장](./openai.md)을 참고한다.

## 지원 범위

Agent Skills는 에이전트에 반복 가능한 절차와 전문 지식, 실행 가능한 보조 도구를 제공하는 디렉터리 규격이다. 최소 단위는 YAML frontmatter와 Markdown 본문을 가진 `SKILL.md`이며, 나머지 파일은 선택 사항이다.

```text
skill-name/
├── SKILL.md          # 필수: 메타데이터와 지침
├── scripts/          # 선택: 실행 코드
├── references/       # 선택: 필요할 때 읽을 상세 문서
├── assets/           # 선택: 템플릿, 이미지, 데이터
└── ...               # 선택: 구현체가 사용할 추가 파일
```

표준이 보장하는 것은 파일 형식과 구성 관례다. 스크립트 언어, 사용 가능한 도구, 승인 방식, skill 검색 위치와 명시적 호출 문법은 호스트 구현이 정한다.

## `SKILL.md` 형식

파일은 첫 부분의 YAML frontmatter와 그 뒤의 Markdown 지침으로 구성한다.

```markdown
---
name: code-review
description: Reviews code for correctness and maintainability. Use when reviewing a change or pull request.
license: MIT
compatibility: Requires git.
metadata:
  author: example-org
  version: "1.0"
allowed-tools: Bash(git:*) Read
---

# Code review

1. Inspect the change.
2. Check correctness, regressions, and tests.
3. Report findings in severity order.
```

### Frontmatter

| 필드 | 필수 | 표준 제약과 의미 |
| --- | --- | --- |
| `name` | 예 | 1~64자. 소문자 `a-z`, 숫자, 하이픈만 허용하며 시작·끝 하이픈과 연속 하이픈은 금지한다. 부모 디렉터리 이름과 같아야 한다. |
| `description` | 예 | 1~1,024자. 무엇을 하는지와 언제 사용하는지를 함께 설명한다. 검색에 도움이 되는 구체적 키워드를 권장한다. |
| `license` | 아니요 | 라이선스 이름 또는 skill에 포함된 라이선스 파일을 가리키는 짧은 문자열이다. |
| `compatibility` | 아니요 | 1~500자. 대상 제품, 필요한 패키지, 네트워크 접근 등 환경 요구 사항이 있을 때만 쓴다. |
| `metadata` | 아니요 | 문자열 키와 문자열 값으로 된 임의 맵이다. 충돌을 줄이기 위해 고유한 키 이름을 권장한다. |
| `allowed-tools` | 아니요 | 미리 승인할 도구를 공백으로 구분한 문자열이다. 실험적 필드라 구현체마다 지원이 다를 수 있다. |

`allowed-tools`까지 포함해 현재 표준이 정의하는 frontmatter 키는 위 여섯 개뿐이다. 다른 키는 제품 확장이므로 이식 가능한 core에 넣기 전에 대상 호스트의 검증 동작을 확인해야 한다.

### Markdown 본문

본문 형식에는 별도 제약이 없다. 일반적으로 다음 내용을 간결하게 둔다.

- 단계별 작업 절차
- 입력과 출력의 예시
- 자주 발생하는 예외와 안전 조건
- 추가 파일의 상대 경로와 읽거나 실행할 조건

skill이 활성화되면 본문 전체가 컨텍스트에 들어가므로, 표준은 `SKILL.md`를 500줄 미만으로 유지하고 상세 자료를 별도 파일로 분리할 것을 권장한다.

## 선택 디렉터리의 역할

| 경로 | 용도 | 작성 기준 |
| --- | --- | --- |
| `scripts/` | 에이전트가 실행할 결정론적 작업 | 자체 완결적이어야 하며 의존성, 오류, 경계 조건을 명확히 처리한다. 지원 언어는 호스트에 따라 다르다. |
| `references/` | API 사양, 도메인 지식, 상세 가이드 | 파일별 주제를 작게 유지하고 필요한 시점을 `SKILL.md`에 명시한다. |
| `assets/` | 결과물에 사용할 템플릿, 이미지, 스키마, 데이터 | 지침이 아니라 복사·변환·참조할 정적 자원을 둔다. |

`SKILL.md`에서 추가 파일을 참조할 때는 skill 루트 기준 상대 경로를 사용한다.

```markdown
API 세부 사항은 [참조 문서](references/api.md)를 읽는다.
검증에는 `scripts/validate.sh`를 실행한다.
```

참조가 다시 다른 참조를 연쇄적으로 요구하지 않도록 한 단계 깊이의 링크를 권장한다.

## Progressive disclosure

표준은 컨텍스트 비용을 줄이기 위해 세 단계 로딩 모델을 권장한다.

| 단계 | 로드하는 내용 | 권장 규모 |
| --- | --- | --- |
| 발견 | 모든 skill의 `name`, `description` | skill당 약 100 tokens |
| 활성화 | 선택된 skill의 `SKILL.md` 전체 | 5,000 tokens 미만 권장 |
| 자원 사용 | `scripts/`, `references/`, `assets/`의 필요한 파일 | 필요할 때만 |

따라서 `description`은 단순 소개가 아니라 라우팅 규칙의 일부다. 핵심 작업과 사용 시점을 앞부분에 쓰고, 본문은 실제 실행 지침에 집중하는 편이 좋다.

## 이식 가능한 작성 기준

Claude Code와 Codex를 함께 대상으로 할 때의 최소 기준은 다음과 같다.

1. `name`과 `description`을 항상 작성하고 표준 길이·문자 규칙을 지킨다.
2. frontmatter에는 표준 여섯 필드만 두거나, 제품별 확장이 필요한 경우 별도 wrapper 또는 vendor 파일로 격리한다.
3. 본문에서 특정 제품의 호출 문법이나 도구 이름을 전제하지 않는다.
4. 환경 요구 사항은 `compatibility`에, 제품이 해석하지 않아도 되는 식별 정보는 `metadata`에 둔다.
5. 추가 파일은 상대 경로로 직접 연결하고, 깊은 참조 체인을 피한다.
6. 외부에서 받은 skill의 스크립트와 `allowed-tools`는 실행 전에 검토한다.

## 검증

표준 문서가 안내하는 reference library인 [`skills-ref`](https://github.com/agentskills/agentskills/tree/main/skills-ref)로 구조와 frontmatter를 검사할 수 있다. 다만 해당 프로젝트 자체는 demonstration 용도이며 production validator로 보증되지 않는다고 명시한다.

```sh
skills-ref validate ./my-skill
```

이 검증은 표준 적합성을 확인할 뿐, Claude Code나 Codex의 제품별 확장과 실행 결과까지 보장하지는 않는다.

## 출처

- [Agent Skills — Specification](https://agentskills.io/specification)
- [Agent Skills — Overview](https://agentskills.io/home)
