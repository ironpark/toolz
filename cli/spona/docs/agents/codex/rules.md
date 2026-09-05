# OpenAI Codex CLI (`codex`) — execpolicy (`.rules`)

> [README](README.md) · [../](../README.md)

출처: <https://learn.chatgpt.com/docs/agent-configuration/rules>
(실험적 기능이며 변경될 수 있음)

Rules는 **어떤 명령을 샌드박스 밖에서 실행할 수 있는지**를 제어합니다.
샌드박스·승인 정책과는 별도의 층입니다.

## 위치

시작 시 여러 레이어를 스캔합니다.

| 위치 | 비고 |
| --- | --- |
| `~/.codex/rules/default.rules` | 사용자 레이어. TUI에서 승인하면 여기에 기록됨 |
| Team Config 위치 | 관리자 배포 |
| `<repo>/.codex/rules/` | 프로젝트 로컬, **신뢰된 경우만** |

`requirements.toml`을 통해 관리자가 제한적인 규칙을 강제할 수 있습니다.

## 형식

`.rules` 파일은 **Starlark**(Python 유사 문법, 부작용 없이 안전하게 평가 가능)로 작성합니다.
주 함수는 `prefix_rule()`입니다.

| 필드 | 설명 |
| --- | --- |
| `pattern` (필수) | 매칭할 명령 인자 목록. 리터럴 또는 `["view", "list"]` 같은 union |
| `decision` (기본 `allow`) | `allow`(프롬프트 없이 실행), `prompt`(매번 승인 요청), `forbidden`(즉시 차단) |
| `justification` (선택) | 사람이 읽을 사유. 승인 프롬프트·거부 메시지에 노출될 수 있음 |
| `match` / `not_match` (선택) | 규칙 로드 시 검증에 쓰이는 예시 명령 |

여러 규칙이 매칭되면 **가장 제한적인 결정**이 적용됩니다: `forbidden` > `prompt` > `allow`.

## 셸 스크립트 처리

`&&`, `||`, `;`, `|` 로 이어진 선형 명령 체인은 안전하게 분리되어 각각 평가됩니다.
반면 리다이렉션·치환·변수·와일드카드를 쓰는 스크립트는 **하나의 호출로 통째 평가**됩니다.

## 검증

```sh
codex execpolicy check --pretty \
  --rules ~/.codex/rules/default.rules \
  -- gh pr view 7888
```

`--rules`는 반복 지정해 여러 파일을 합칠 수 있고, 매칭된 규칙과 최종(가장 엄격한) 결정을
JSON으로 출력합니다. spona가 권한 프리셋을 내보낼 때 **적용 결과를 사전 검증하는 수단**으로
쓸 수 있습니다.

`codex exec --ignore-rules` 는 user/project `.rules` 로드를 건너뜁니다.
