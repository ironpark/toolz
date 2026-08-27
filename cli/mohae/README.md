# mohae

`mohae`는 AI 에이전트와 그 에이전트가 사용하는 도구(CLI, MCP 서버, 스킬)를 재현 가능한
환경에서 실행하고 비교·평가하는 CLI입니다.

에이전트에게 도구를 쥐여 주고 나면 "이 도구를 잘 쓰는가", "지침 문구를 이렇게 바꾸면
더 나아지는가" 같은 질문이 생깁니다. 한 번 실행해 보고 판단하기에는 결과가 매번 달라서,
같은 조건을 반복하고 한 가지 조건만 바꿔 비교할 도구가 필요합니다. `mohae`는 그 반복을
설정 파일로 고정합니다.

> [!NOTE]
> [`planr`](../planr/)의 내부 평가 하네스를 일반화해 독립 프로젝트로 분리한 것입니다.
> 현재는 CLI 골격과 설정 파일 검증까지 동작하며, 실제 실행은 구현 중입니다.

## 어떻게 동작하나

한 번의 실행(trial)은 설정 파일 하나가 정의합니다.

1. **격리** — `workspace.source`를 임시 디렉터리로 복사합니다. 원본은 절대 수정되지
   않으므로 같은 설정의 두 실행은 동일한 상태에서 시작합니다.
2. **준비** — `workspace.init_script`로 의존성을 빌드하거나 데이터를 심고,
   `workspace.agent_md`를 `AGENTS.md`로 설치합니다.
3. **실행** — 시작 프롬프트를 **한 번만** 보내고 개입하지 않습니다. 에이전트가 스스로
   완료를 판단해야 하며, 그동안 대화·명령 실행·실패·토큰 사용량을 기록합니다.
4. **채점** — `verify.script`가 끝난 워크스페이스를 검사합니다. 워크스페이스 **밖**에서
   실행되고 안으로 복사되지도 않으므로, 에이전트가 검사 항목에 맞춰 결과를 꾸밀 수
   없습니다.
5. **리포트** — 성공 여부, 토큰 종류별 사용량, 소요 시간, 실패한 명령을 리포트로 남깁니다.

프롬프트를 워크스페이스에 두지 않는 이유는, 디스크에서 다시 읽을 수 있는 과제 명세가
아니라 대화로 받은 요청만으로 일하는 실제 상황을 재현하기 위해서입니다.

## 설정 파일

`mohae.config.yaml` 하나가 환경 하나입니다. 여러 개를 두고 glob으로 함께 실행할 수
있습니다. 모든 경로는 **설정 파일 위치 기준**으로 해석되므로 어느 디렉터리에서 실행해도
동일하게 동작합니다.

```yaml
name: kvstore-codex
description: 로그 기반 KV 저장소 과제를 codex로 평가

agent:
  type: codex # claude-code | codex | custom-cli
  model: gpt-5.6-luna
  reasoning: medium

workspace:
  source: ./fixture # 격리 디렉터리로 복사할 원본
  init_script: ./init.sh
  agent_md: ./AGENTS.md # 워크스페이스에 AGENTS.md로 설치
  git: true

prompt:
  file: ./PROMPT.md # 또는 text: "..."

target_cli:
  command: planr # 에이전트가 사용할 도구
  build: go build -o bin/planr ./cli/planr

verify:
  script: ./verify.sh

limits:
  timeout_seconds: 300
  max_turns: 30

report:
  dir: .mohae/reports
  formats: [terminal, json]
```

`AGENTS.md`를 픽스처 안이 아니라 밖에 두는 것이 중요합니다. 과제와 무관한 문서이므로
설정 파일마다 사본을 두면 문구를 고칠 때 사본이 서로 어긋나고, 두 실행이 더 이상 같은
것을 측정하지 않게 됩니다.

### 검증 스크립트

`verify.sh`는 `MOHAE_WORKSPACE` 환경 변수로 완료된 워크스페이스 경로를 받습니다.
검사 결과를 다음 형식으로 출력하면 리포트가 표로 정리합니다.

```text
CHECK<TAB>이름<TAB>PASS|FAIL<TAB>비고
```

## 사용법

```sh
mohae init --with-scripts --with-agent-md   # 템플릿 생성
mohae verify --check-scripts                # 실행 전 점검
mohae run                                   # 실행과 리포트
```

### `mohae run`

설정 파일에 정의된 환경을 실행하고 지표를 수집합니다. 인자를 주지 않으면
`./mohae.config.yaml`을 사용하며, glob으로 여러 설정을 함께 실행할 수 있습니다.

```sh
mohae run
mohae run 'trials/*.config.yaml' --concurrency 4
mohae run --agent claude-code --detailed-tokens
```

| 옵션                     | 설명                                                       |
| ------------------------ | ---------------------------------------------------------- |
| `-a, --agent <TYPE>`     | 에이전트 종류 오버라이드                                   |
| `-p, --prompt <TEXT>`    | 시작 프롬프트를 인라인으로 대체                            |
| `-P, --prompt-file`      | 시작 프롬프트를 파일로 대체                                |
| `--agent-md <PATH>`      | 설치할 `AGENTS.md` 대체                                    |
| `--init-script <PATH>`   | 환경 구성 스크립트 대체                                    |
| `--verify-script <PATH>` | 검증 스크립트 대체                                         |
| `-m, --mcp-config`       | MCP 서버 설정 주입                                         |
| `--target-cli <CMD>`     | 테스트 대상 CLI 대체                                       |
| `-o, --output <FORMAT>`  | `terminal`, `json`, `markdown`, `html`                      |
| `--report-dir <DIR>`     | 리포트 저장 위치 (기본 `.mohae/reports`)                   |
| `--show-dialogue`        | 대화 내용을 터미널로 실시간 출력                           |
| `--detailed-tokens`      | 입력·출력·캐시 읽기/쓰기로 토큰을 나눠 출력                |
| `-t, --timeout <SEC>`    | 실행 하나당 제한 시간 (기본 300)                           |
| `--max-turns <NUM>`      | 최대 대화 턴 수 (기본 30)                                  |
| `--fail-fast`            | 검증 실패나 명령 에러 발생 시 즉시 중단                    |
| `-c, --concurrency`      | 동시에 실행할 trial 수 (기본 1)                            |

오버라이드는 설정 파일을 고치지 않고 이번 실행에만 적용됩니다. 같은 설정을 조건만 바꿔
반복할 수 있어야 A/B 비교가 성립하기 때문입니다.

### `mohae compare`

두 조건을 대조 실행합니다. 에이전트 실행은 결정적이지 않으므로 한 쌍만 비교해서는 실제
차이와 잡음을 구분할 수 없고, 그래서 `--repeat`이 기본 3회입니다.

```sh
mohae compare --a ./a.config.yaml --b ./b.config.yaml
mohae compare --a ./agents-en.md --b ./agents-strict.md --target agent-md -n 5
```

| 옵션                  | 설명                                                        |
| --------------------- | ----------------------------------------------------------- |
| `--a`, `--b`          | 기준군과 대조군 (설정 파일 경로 또는 비교 대상 값)          |
| `--target <FIELD>`    | `auto`, `prompt`, `agent-md`, `agent`, `mcp`, `config`       |
| `-n, --repeat <NUM>`  | 반복 횟수 (기본 3)                                          |
| `--metric <TYPE>`     | `success-rate`, `tokens`, `cost`, `duration`                |
| `--diff-only`         | 차이가 발생한 대화 턴과 결과만 출력                         |

### `mohae verify`

실행 없이 설정을 정적 검사합니다. 토큰을 쓰기 전에 경로 오타나 실행 권한 누락을 잡는
것이 목적입니다.

```sh
mohae verify --check-scripts --check-agent-md --strict
```

| 옵션               | 설명                                                    |
| ------------------ | ------------------------------------------------------- |
| `--check-mcp`      | MCP 서버 응답과 도구 목록 확인                          |
| `--check-scripts`  | 스크립트 구문 오류(`bash -n`)와 실행 권한(`+x`) 검사    |
| `--check-agent-md` | `AGENTS.md` 내용 유효성 검사                            |
| `--strict`         | 경고도 실패로 처리                                      |

경고와 실패를 구분합니다. `verify.script`가 없는 설정은 합격/불합격 판정 없이도 실행은
되므로 경고이고, 경로가 존재하지 않으면 실패입니다.

### `mohae init`

```sh
mohae init                                   # ./mohae.config.yaml
mohae init trials/kvstore --template cli-skill --with-scripts
```

템플릿은 `basic`, `mcp-server`, `cli-skill`, `multi-agent`입니다. 무엇을 테스트 대상으로
두는지만 다르고, 격리·프롬프트·검증이라는 흐름은 모두 같습니다.

### `mohae web`

리포트를 읽는 로컬 대시보드입니다(구현 예정). 대화 전문과 워크스페이스 내용이 그대로
담기므로 기본 바인딩은 `127.0.0.1`입니다.

### `mohae report`

과거 리포트를 다른 형식으로 다시 출력하거나, 디렉터리 전체의 토큰·비용·성공률을
집계합니다(구현 예정).

## 구현 상태

| 명령      | 상태                                              |
| --------- | ------------------------------------------------- |
| `init`    | 동작 — 설정·스크립트·`AGENTS.md` 템플릿 생성      |
| `verify`  | 동작 — 경로·스크립트·`AGENTS.md` 검사 (MCP는 예정) |
| `run`     | 설정 로딩과 오버라이드까지 동작, 실행은 구현 중   |
| `compare` | 인자 검증까지 동작                                |
| `report`  | 인자 검증까지 동작                                |
| `web`     | 미구현                                            |

## 개발

Go 1.26.3 이상이 필요합니다.

```sh
cd cli/mohae
go test ./...
go build -o mohae .
```
