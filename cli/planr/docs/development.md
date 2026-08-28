# 개발 및 평가

[README로 돌아가기](../README.md) · [명령어 가이드](commands.md) · [문서 규격](document-format.md)

이 문서는 `planr` 자체를 수정하고 검증하는 기여자를 위한 내용입니다. 일반적인 사용에는
필요하지 않습니다.

## 파일 읽기 경로

planr이 읽는 모든 문서 — 설정, plan 디렉터리, phase 문서, 초안 — 는
[`internal/vfs`](../internal/vfs)를 지나갑니다. `vfs.ReadFile`, `vfs.ReadDir`,
`vfs.Stat`은 호스트 경로를 받아 `io/fs` 이름으로 옮긴 뒤 현재 `fs.FS`에서 읽고,
`vfs.Use(fsys)`로 테스트가 인메모리 트리를 끼워 넣을 수 있습니다. 기본값은 os 패키지로
곧장 넘기는 `hostFS`라 실제 실행에는 변환 비용이 없습니다.

쓰기·잠금·git 접근은 `io/fs`가 읽기 전용이므로 os 패키지에 그대로 둡니다. 새 읽기를
추가할 때 `os.ReadFile` 대신 `vfs.ReadFile`을 쓰면 이 경계가 유지됩니다.

## 로컬 검증

Go 모듈은 [`cli/planr`](..)에 독립적으로 구성되어 있습니다.

```sh
cd cli/planr
go test ./...
go vet ./...
```

Python 실행기 테스트는 저장소 루트에서 실행합니다.

```sh
uv run --with pytest --project cli/planr/scripts \
  python -m pytest cli/planr/scripts -q
```

## 개발 실행기

실행기 코드는 [`scripts/`](../scripts)에 있는 uv 프로젝트이며, 진입점은
[`scripts/main.py`](../scripts/main.py) 하나입니다.

| 명령 | 하는 일 | 필요한 도구 |
| --- | --- | --- |
| `main.py codex` | 격리 저장소에서 Codex 평가 실행 | uv, Go, Git, Codex 로그인 |
| `main.py codex variants` | 픽스처별 요청·지침 변형 조회 | Python |
| `main.py codex analyze <dir>` | 이전 실행 결과 재분석 | Python |
| `main.py codex clean` | Codex 실행 디렉터리와 임시 작업공간 삭제 | Python |

Codex SDK는 평가를 시작할 때만 불러옵니다. 따라서 `variants`, `analyze`, `clean`은
SDK 없이도 실행할 수 있습니다. 공통 준비·정리와 하네스 설정 처리는
[`scripts/common.py`](../scripts/common.py), Codex 세션은
[`scripts/codex.py`](../scripts/codex.py), 결과 분석은
[`scripts/analyze.py`](../scripts/analyze.py)에 있습니다.

## 실행 디렉터리와 격리

모든 실행은 `cli/planr/run/` 아래에 `<UTC timestamp>-<runner>` 디렉터리를 만들고
산출물을 보관합니다. 같은 초에 이름이 겹치면 번호가 붙습니다.

```text
cli/planr/run/
├── 20260826-123851-codex/
└── 20260826-124012-codex-regex/
```

Codex가 수정하는 작업공간은 `run/` 바깥의 시스템 임시 디렉터리에 만듭니다. 에이전트가
상위 경로를 탐색해 자신의 평가 리포트나 transcript를 읽지 못하게 하기 위함입니다.
작업공간 경로는 실행 디렉터리의 `metadata.env`에 기록되며 `codex clean`이 이 값을 따라
실행 결과와 작업공간을 함께 정리합니다. 자동으로 삭제하지 않으므로 실패 후 상태를 직접
검사할 수 있습니다.

## 픽스처

픽스처는 [`fixtures/`](../fixtures)에 있고 역할과 파일 계약은
[`fixtures/MANIFEST.yaml`](../fixtures/MANIFEST.yaml)과
[`fixtures/HARNESS.json`](../fixtures/HARNESS.json)에 정의되어 있습니다.

| 픽스처 | 목적 |
| --- | --- |
| `codex-harness` | 기존의 작은 Go 프로젝트를 수정하는 기본 평가 |
| `codex-greenfield` | 빈 저장소에서 다중 명령 할 일 CLI를 만드는 평가 |
| `codex-regex` | 표준 regexp 없이 정규식 엔진을 구현하는 깊이 중심 평가 |
| `codex-kvstore` | 로그 복구, compaction, 동시 쓰기 불변식을 다루는 평가 |

실행기는 픽스처를 실행별 작업공간으로 복사한 뒤 그 안에서만 변경합니다. 새 픽스처를
추가하면 `MANIFEST.yaml`에도 용도와 내용을 추가하고, 기계 판독 가능한 빌드·관찰·완료
규칙은 `HARNESS.json`에 둡니다.

### 평가 전용 파일

`FIXTURE.` 접두사 파일은 평가 설정이며 에이전트 작업공간에 그대로 복사되지 않습니다.

| 파일 | 처리 방식 |
| --- | --- |
| `FIXTURE.PROMPT.<variant>.md` | 선택된 파일을 첫 사용자 요청으로만 전달 |
| `FIXTURE.AGENTS.<variant>.md` | 같은 이름의 공용 지침이 있을 때 픽스처 전용으로 덮어씀 |
| `FIXTURE.TEST.sh` | 세션 종료 후 작업공간 밖에서 인수 검사로 실행 |
| 그 외 파일 | 작업공간에 복사 |

공용 에이전트 지침은 [`fixtures/agents/`](../fixtures/agents)에 둡니다. 요청은 과제에
종속되므로 각 픽스처에, planr 사용 지침은 과제와 무관하므로 공용 디렉터리에 둡니다.
요청 본문에 planr 워크플로를 반복해서 적지 않아야 에이전트가 저장소 지침을 발견하고
따르는지 측정할 수 있습니다.

## checkout 출시 시나리오

완료·진행·대기·부분 완료 plan이 섞인 상태에서 `status`, `overview`, `notes`가 무엇을
보여주는지는 Go 테스트로 검증합니다. 에이전트가 필요 없는 순수 plan 생성 시나리오라
픽스처와 Python 실행기 대신 [`cli/scenario_test.go`](../cli/scenario_test.go)에 있습니다.

```sh
cd cli/planr
go test ./cli/ -run Scenario -v
```

테스트는 임시 Git 저장소에 다음 상태를 실제 명령 호출로 만듭니다.

- 완료된 인증 기반 plan
- 진행 중 checkout plan
- checkout을 기다리는 결제 plan
- 일부 phase만 완료된 rollout plan
- `status`에서 숨겨지는 무관한 완료 plan

다섯 plan은 모두 [`internal/plantest/fixtures/checkout-v2.md`](../internal/plantest/fixtures/checkout-v2.md)
초안 본문 하나를 이름과 의존성만 바꿔 재사용합니다. 픽스처는 `embed`로 묶여
`plantest.Fixtures()`가 `io/fs` 트리로 돌려주므로 테스트가 파일 경로가 아니라
`fs.FS`를 읽습니다. 의존성은 공개 인터페이스인 초안 frontmatter로 입력하고 상태는
`planr phase` 명령으로 변경합니다. 생성된 내부 문서를 직접 치환하지 않으므로 실제
CLI가 거부할 상태를 억지로 만들지 않으며, 초안 계약이 바뀌면 시나리오도 명확하게
실패합니다.

## Codex 평가

처음 실행하기 전에 고정된 Python 의존성을 설치합니다.

```sh
uv sync --project cli/planr/scripts
```

Codex 인증은 로컬 설정을 사용합니다. 하네스는 승인을 자동으로 거부하고 파일 접근을
격리 작업공간에 한정합니다. 아래 예시는 짧은 별칭을 사용합니다.

```sh
alias planr-codex='uv run --locked --project cli/planr/scripts python cli/planr/scripts/main.py codex'

planr-codex
planr-codex --fixture codex-greenfield
planr-codex --model gpt-5.6-luna --reasoning low
planr-codex --language ko
planr-codex --timeout 7200
planr-codex --dry-run
planr-codex --quiet

planr-codex variants
planr-codex analyze cli/planr/run/20260826-123851-codex
planr-codex clean
```

기본 모델은 `gpt-5.6-luna`, reasoning은 `medium`, 전체 세션 timeout은 3600초입니다.
`--dry-run`은 Codex를 호출하지 않고 작업공간, 빌드와 산출물 경로를 점검합니다.

### 언어와 A/B 변형

실행 언어는 다음 우선순위로 정합니다.

1. `--language en|ko` 또는 `PLANR_HARNESS_LANGUAGE`
2. 픽스처 `.planr.yaml`의 `language`
3. planr 기본값 `en`

선택된 언어는 요청·지침의 기본 변형과 작업공간 `.planr.yaml`에 모두 반영됩니다. 그렇지
않으면 지침 언어와 planr가 생성하는 문서 언어가 달라져 언어가 아닌 설정 모순을 측정하게
됩니다.

요청과 지침은 독립적으로 바꿀 수 있습니다.

- `--prompt <variant>` 또는 `PLANR_HARNESS_PROMPT`: 픽스처의 요청 변형
- `--agents <variant>` 또는 `PLANR_HARNESS_AGENTS`: 공용 또는 픽스처 전용 지침 변형

```sh
planr-codex variants
planr-codex --agents en --language ko
planr-codex --fixture codex-regex --agents strict
```

A/B 비교에서는 픽스처, 모델, reasoning과 언어를 고정하고 요청 또는 지침 한쪽만 바꿉니다.
선택값은 `metadata.env`와 `REPORT.md`에 남습니다.

### 세션 원칙과 진행 로그

하네스는 요청을 한 번만 보내며 후속 메시지로 완료를 유도하지 않습니다. `--timeout`은
각 모델 호출이 아니라 전체 작업 시간입니다. 실행 중 진행 로그는 stderr, 최종 결과 경로는
stdout에 출력되므로 stdout을 파일로 리디렉션해도 로그는 계속 볼 수 있습니다.

```text
[   0:00] preparing isolated repository from fixture codex-harness
[   0:03] thread th_01J started on gpt-5.6-luna (reasoning low)
[   0:24]   think  Read AGENTS.md, then plan the work
[   0:31]   cmd  planr new json-output --description "..."
[   1:02]   edit  main.go, main_test.go
[   1:14]   cmd  go test ./... (exit 1)
[41:52] session completed in 41:49 · 87 items · 268.4k tokens
[42:18] final go test exit 0; writing report
```

세부 SDK 이벤트는 실행 중에도 `session.jsonl`을 `tail -f`해서 볼 수 있습니다.

### 산출물

| 파일 | 내용 |
| --- | --- |
| `REPORT.md` | 완료 여부, 검증 결과, 토큰, 관찰된 planr 사용과 개선 신호 |
| `transcript.md` | 대화와 명령 추출본 |
| `session.jsonl` | SDK 원본 알림과 정규화된 결과 |
| `session.prompt.md` | 에이전트에게 보낸 요청 |
| `metrics.json` | 모델·설정 간 비교용 수치 |
| `metadata.env` | fixture, 언어, 변형, 모델, workspace 등 실행 조건 |
| `state/` | 최종 Git 상태, 테스트와 fixture 검사 결과 |

실패를 재분석할 때는 기존 작업공간을 직접 바꾸기보다 `codex analyze <run-dir>`를 먼저
실행해 같은 원본 이벤트에서 리포트를 다시 생성하세요.
