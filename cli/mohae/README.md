# mohae

`mohae`는 AI 에이전트와 그 에이전트가 사용하는 도구(CLI, MCP 서버, 스킬)를 재현 가능한
환경에서 실행하고 비교·평가하는 CLI입니다.

에이전트에게 도구를 쥐여 주고 나면 "이 도구를 잘 쓰는가", "지침 문구를 이렇게 바꾸면
더 나아지는가" 같은 질문이 생깁니다. 한 번 실행해 보고 판단하기에는 결과가 매번 달라서,
같은 조건을 반복하고 한 가지 조건만 바꿔 비교할 도구가 필요합니다. `mohae`는 그 반복을
설정 파일로 고정합니다.

> [!NOTE]
> [`planr`](../planr/)의 내부 평가 하네스를 일반화해 독립 프로젝트로 분리한 것입니다.
> `init`·`verify`·`run`(실행·채점·리포트)이 동작하며, `compare`·`report`·`web`은
> 아직 인자 검증까지입니다. 자세한 범위는 아래 [구현 상태](#구현-상태)를 보세요.

## 어떻게 동작하나

한 번의 실행(trial)은 설정 파일 하나가 정의합니다.

1. **격리** — `workspace.source`를 임시 디렉터리로 복사합니다. mohae는 원본을
   수정하지 않으므로 같은 설정의 두 실행은 동일한 상태에서 시작합니다.
   `container`를 지정하면 이후 단계가 호스트가 아니라 그 이미지 안에서 실행되어,
   시작 상태뿐 아니라 툴체인까지 고정됩니다.
2. **준비** — `workspace.init_script`로 의존성을 빌드하거나 데이터를 심고,
   `workspace.agent_md`를 `AGENTS.md`로 설치합니다.
3. **실행** — `prompts`를 순서대로 보냅니다. 각 프롬프트는 앞 턴이 끝난 뒤에 전송되고,
   `when` 조건이 붙어 있으면 조건이 참일 때만 보냅니다. 그 사이에는 개입하지 않으므로
   에이전트가 스스로 완료를 판단해야 하며, 그동안 대화·명령 실행·실패·토큰 사용량을
   기록합니다.
4. **후처리** — `hooks.after`가 에이전트 종료 직후 선택한 scope에서 명령을 실행합니다.
5. **채점** — `verify.commands`가 워크스페이스 밖에서 결과를 검사합니다.
6. **보존** — `artifacts`를 복사하고 결과와 사용량을 리포트로 남긴 뒤 임시
   워크스페이스를 정리합니다.

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
  effort: medium

workspace:
  source: ./fixture # 격리 디렉터리로 복사할 원본
  exclude: [FIXTURE.*, tmp/**] # 에이전트에게 보여 주지 않을 source 내부 glob
  init_script: ./init.sh
  agent_md: ./AGENTS.md # 워크스페이스에 AGENTS.md로 설치
  git: true

container: # 생략하면 호스트에서 실행. image나 build가 있어야 켜집니다
  image: golang:1.26 # 또는 build: ./docker (Dockerfile이 있는 디렉터리)
  runtime: auto # auto | docker | podman
  scope: setup # setup | full
  network: none
  user: host # host | root | uid:gid
  mounts:
    - source: ~/.codex
      target: /mohae-home/.codex
      read_only: true
  env:
    CODEX_HOME: /mohae-home/.codex

prompts: # 대화. 순서대로 전송되며, 둘 이상이면 멀티턴
  - file: ./PROMPT.md
  - text: 빌드가 깨져 있습니다. 멈추기 전에 고치세요.
    when: sh("go build ./...") != 0 # 조건이 참일 때만 전송

skills: # 워크스페이스에 설치할 스킬. agents로 대상 에이전트 제한
  - path: ./skills/commit
    agents: [claude-code]

mcp: # 연결할 MCP 서버. agents 생략 시 모든 에이전트에 제공
  - name: context7
    config: ./mcp.json
    agents: [claude-code, codex]

hooks:
  after: # 에이전트 종료 후, 검증 전에 실행
    - ./finalize.sh
    - run: ./publish-summary.sh
      scope: outside # workspace | outside

verify:
  commands: # 셸 명령을 순서대로 실행. 각각 종료 코드 0이면 합격, 출력 형식은 자유
    - ./verify.sh
    - test -f "$MOHAE_WORKSPACE/README.md"

artifacts: # 검증 후 report.dir에 보존할 workspace 상대 경로 또는 glob
  - plans-active/**
  - .harness/planr-events.log

limits:
  timeout_seconds: 300

report:
  dir: .mohae/reports
  formats: [terminal, json]
```

### 컨테이너 격리

`container`가 있으면 trial은 호스트가 아니라 컨테이너 안에서 실행됩니다. 워크스페이스
복사만으로는 **에이전트가 무엇에서 시작하는지**만 고정될 뿐, 그것을 빌드하는 툴체인과
채점하는 명령은 여전히 그 머신에 깔린 것을 씁니다. 이미지는 그 나머지를 고정합니다.

`scope`가 경계를 어디에 둘지 정합니다.

| scope             | 컨테이너 안                                 | 호스트     | 이미지 요구사항        |
| ----------------- | ------------------------------------------- | ---------- | ---------------------- |
| `setup` (기본값)  | `init_script`, `hooks.after`, `verify`, `sh()` | 에이전트   | 과제의 툴체인          |
| `full`            | 위 전부 + 에이전트                          | —          | 툴체인 + 에이전트 CLI  |

`setup`은 채점 환경의 재현성만 필요할 때 쓰는 값이고, 이미지에 에이전트를 넣지 않아도
됩니다. `full`은 에이전트까지 컨테이너가 묶으므로 [알려진 한계](#알려진-한계)의
`claude-code` 샌드박스 문제를 해소하지만, 이미지가 에이전트 CLI를 담고 있어야 하고
로그인 자격 증명을 `mounts`로 넣어 줘야 합니다.

동작에 관해 알아 둘 것:

- **경로.** trial의 임시 디렉터리는 컨테이너 안에서 항상 `/mohae`에 마운트됩니다.
  워크스페이스는 `/mohae/workspace`, scratch는 `/mohae/scratch`입니다. `$MOHAE_WORKSPACE`는
  **그 명령이 보는 경로**로 채워지므로, `scope: setup`에서는 에이전트가 호스트 경로를,
  검증 명령이 컨테이너 경로를 받습니다. 두 이름은 같은 파일을 가리킵니다.
- **`$HOME`.** `/mohae/home`이 기본값입니다. 호스트 사용자로 실행되는 컨테이너에는
  대개 passwd 항목도 홈도 없어서, 이것이 없으면 패키지 매니저와 에이전트 CLI가
  쓰기에 실패합니다. `container.env`로 덮어쓸 수 있습니다.
- **파일 소유권.** `user: host`(기본값)는 호스트의 uid/gid로 실행하므로 trial이 만든
  파일을 그대로 읽고 지울 수 있습니다. rootless podman에는 `--userns=keep-id`가 함께
  붙습니다. `user: root`는 호스트에 root 소유 파일을 남기고 워크스페이스 정리를
  실패시킵니다.
- **스크립트.** `init_script`는 설정 파일 옆에 있어 컨테이너가 볼 수 없으므로 trial의
  디렉터리로 복사한 뒤 실행합니다.
- **취소.** 턴 제한 시간이나 `limits.timeout_seconds`가 만료되면 컨테이너 안에서 그
  명령이 시작한 프로세스 전체를 죽입니다. 워크스페이스 정리 시 컨테이너도 제거됩니다.
- **런타임.** `auto`는 docker, podman 순으로 찾습니다. 설정이 지정한 런타임이 없으면
  호스트로 물러서지 않고 실패합니다 — 격리되지 않은 실행이 격리된 것으로 기록되는
  편이 더 나쁘기 때문입니다. `mohae verify`가 실행 전에 이를 검사합니다.
- **비용.** trial마다 컨테이너를 하나 띄웁니다. `build`는 매번 호출하지만 런타임의
  레이어 캐시가 반복 빌드를 감당합니다.

### 프로파일

`profiles`는 설정의 부분집합에 이름을 붙인 것입니다. `--profile <이름>`으로
선택하면 프로파일이 선언한 섹션이 기본 설정을 **통째로** 덮어씁니다(필드 단위
병합이 아님). 하나의 파일로 여러 변형 — 다른 에이전트, 더 짧은 제한 — 을 기술하고
실행 때 골라 쓸 수 있습니다.

```yaml
profiles:
  claude:
    agent:
      type: claude-code
      model: claude-opus-5
  quick:
    limits:
      timeout_seconds: 60
```

```sh
mohae run --profile claude --profile quick # 순서대로 적용, 뒤가 우선
```

프로파일이 먼저 적용되고 `--agent` 같은 필드 플래그가 그 위에 적용되므로,
프로파일로 고른 구성을 플래그로 미세 조정할 수 있습니다.

### 프롬프트와 실행 조건

`prompts`는 대화 전체입니다. 항목이 하나면 단일 턴, 여럿이면 멀티턴 실행이 됩니다.
각 항목은 `text:` 또는 `file:` 중 하나를 가지며, 한 줄짜리 프롬프트는 문자열로 줄여
쓸 수 있습니다.

```yaml
prompts:
  - file: ./PROMPT.md
  - 이제 테스트를 작성하세요 # text: 의 축약형
```

`when:`을 붙이면 그 프롬프트는 조건이 참일 때만 전송됩니다. 후속 지시가 항상 필요한
것은 아니기 때문입니다 — "빌드가 깨졌으면 고치라고 한 번 더 말한다" 같은 분기를 설정
파일 하나로 표현할 수 있고, 조건이 거짓인 실행은 그 턴을 건너뛰므로 토큰도 쓰지
않습니다.

조건은 [expr](https://github.com/expr-lang/expr) 표현식이며 불리언으로 평가되어야
합니다. 설정을 읽는 시점에 컴파일되므로, 오타는 토큰을 쓰기 전에 잡힙니다.

`name:`으로 프롬프트에 이름을 붙이면 뒤의 프롬프트가 `after:`로 참조할 수
있습니다. 의존한 프롬프트가 실제로 전송되지 않았다면(조건이 거짓이었다면) 그
프롬프트도 함께 건너뜁니다 — 맥락 없는 후속 지시가 도착하는 일을 막기 위해서입니다.
`after`는 앞서 정의된 `name`만 참조할 수 있고, 중복 `name`이나 앞으로의 참조는
설정을 읽는 시점에 거부됩니다.

```yaml
prompts:
  - name: build
    text: 빌드를 고치세요
    when: sh("go build ./...") != 0
  - text: 고친 빌드에 대한 테스트를 추가하세요
    after: [build] # build가 전송되지 않은 실행에서는 이 턴도 건너뜀
```

`timeout_seconds:`를 붙이면 그 프롬프트가 전송된 순간부터 시간을 재고, 제한을
넘긴 턴은 자동으로 취소됩니다. 지정하지 않은 프롬프트는 자체 제한 없이
`limits.timeout_seconds`(trial 전체 제한)만 적용받습니다.

```yaml
prompts:
  - file: ./PROMPT.md
    timeout_seconds: 120 # 이 턴만의 제한. 전송 시점부터 카운트
  - 이제 테스트를 작성하세요
```

| 이름                 | 뜻                                            |
| -------------------- | --------------------------------------------- |
| `turn`               | 이 프롬프트의 순서 (1부터)                    |
| `previous`           | 직전 응답 (첫 턴 전에는 빈 문자열)            |
| `responses`          | 지금까지의 모든 응답, 오래된 것부터           |
| `elapsed_seconds`    | 지금까지 소요된 시간                          |
| `timed_out`          | 제한 시간에 이미 걸렸는지                     |
| `exists(path)`       | 워크스페이스 기준 경로가 존재하는지           |
| `read(path)`         | 파일 내용, 읽을 수 없으면 `""`                |
| `sh(cmd)`            | 워크스페이스에서 명령을 실행하고 종료 코드 반환 |

`exists`/`read`/`sh`는 에이전트가 **말한 것**이 아니라 **실제로 한 것**을 보고 분기하기
위한 것입니다.

```yaml
prompts:
  - file: ./PROMPT.md
  - text: 테스트가 없습니다. 추가하세요.
    when: not exists("main_test.go")
  - text: 빌드가 깨져 있습니다. 멈추기 전에 고치세요.
    when: sh("go build ./...") != 0
  - text: 요약해 주세요.
    when: previous contains "완료" and not timed_out
```

`AGENTS.md`를 픽스처 안이 아니라 밖에 두는 것이 중요합니다. 과제와 무관한 문서이므로
설정 파일마다 사본을 두면 문구를 고칠 때 사본이 서로 어긋나고, 두 실행이 더 이상 같은
것을 측정하지 않게 됩니다.

### 검증 스크립트

`verify.commands`에는 셸 명령을 여러 개 나열할 수 있고, 에이전트가 멈춘 뒤 순서대로
실행됩니다. 각 명령은 `MOHAE_WORKSPACE` 환경 변수로 완료된 워크스페이스 경로를
받고, **종료 코드가 판정입니다** — 0이면 합격, 그 외는 불합격. 출력 형식은 정해져
있지 않습니다: 사람이 읽는 데 도움이 되는 내용을 자유롭게 출력하면 mohae가 그대로
기록합니다. 한 줄짜리 검사는 명령으로 바로 쓰고, 복잡한 채점은 스크립트 파일을
만들어 명령으로 호출하면 됩니다.

## 사용법

```sh
mohae init --all                            # 템플릿 생성
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
mohae run -p '구현하세요' -p '이제 테스트를 작성하세요'
mohae run -p '구현하세요' -p '빌드를 고치세요' \
  --prompt-when '' --prompt-when 'sh("go build ./...") != 0'
mohae run -p 'file://PROMPT.md' -p '이제 테스트를 작성하세요'
```

`--prompt`는 설정의 대화를 **덧붙이지 않고 통째로 대체**합니다. 설정에 무엇이 들어
있든 `--prompt`의 의미가 같아야 하기 때문입니다. 값이 `file://`로 시작하면 그 뒤의
경로에서 턴 내용을 읽어 오며, 인라인 텍스트와 파일 턴을 자유롭게 섞어도 플래그를 적은
순서 그대로 전송됩니다. 상대 경로는 설정 파일이 아니라 현재 작업 디렉터리를 기준으로
해석합니다. `--prompt-when`은 같은 위치의 프롬프트에 붙으며, 조건을 걸지 않을 자리는
`''`로 비워 둡니다.

| 옵션                     | 설명                                                       |
| ------------------------ | ---------------------------------------------------------- |
| `--profile <NAME>`       | 설정의 프로파일 적용 (반복 가능, 뒤가 우선)                |
| `-a, --agent <TYPE>`     | 에이전트 종류 오버라이드                                   |
| `-p, --prompt <TEXT>`    | 대화 전체를 대체 (반복 가능, 한 번이 한 턴). `file://PATH`는 파일에서 읽음 |
| `--prompt-when <EXPR>`   | 같은 순서의 프롬프트에 붙일 실행 조건 (반복 가능)          |
| `--agent-md <PATH>`      | 설치할 `AGENTS.md` 대체                                    |
| `--init-script <PATH>`   | 환경 구성 스크립트 대체                                    |
| `--container-image <IMG>`| 이 이미지 안에서 trial 실행                                |
| `--container-scope`      | `setup`(기본) 또는 `full`                                  |
| `--verify-command <CMD>` | 검증 명령 목록 대체 (반복 가능)                            |
| `-m, --mcp-config`       | MCP 서버 설정 주입                                         |
| `-o, --output <FORMAT>`  | `terminal`, `json`, `markdown`, `html`                      |
| `--report-dir <DIR>`     | 리포트 저장 위치 (기본 `.mohae/reports`)                   |
| `--show-dialogue`        | 대화 내용을 터미널로 실시간 출력                           |
| `--detailed-tokens`      | 입력·출력·캐시 읽기/쓰기로 토큰을 나눠 출력                |
| `-t, --timeout <SEC>`    | 실행 하나당 제한 시간 (기본 300)                           |
| `--fail-fast`            | 검증 실패나 명령 에러 발생 시 즉시 중단                    |
| `-c, --concurrency`      | 동시에 실행할 trial 수 (기본 1)                            |

오버라이드는 설정 파일을 고치지 않고 이번 실행에만 적용됩니다. 같은 설정을 조건만 바꿔
반복할 수 있어야 A/B 비교가 성립하기 때문입니다.

#### 워크스페이스 준비

`workspace.source`를 임시 디렉터리로 복사하되 `workspace.exclude`와 일치하는 항목은
제외합니다. 슬래시(`/`)가 없는 패턴은 모든 깊이의 파일명에 적용되고 `**`는 여러 디렉터리
깊이를 가로지릅니다. 예를 들어 `FIXTURE.*`는 위치와 관계없이 같은 접두사의 파일을,
`generated/**`는 디렉터리 전체를 제외합니다. 빈 패턴, 절대 경로, `..`가 포함된 경로는
설정을 읽을 때 거부합니다.

복사 후 `AGENTS.md`와 해당 에이전트용 skill을 설치하고 `workspace.init_script`를
실행합니다. `workspace.git`이 켜져 있으면 준비가 끝난 상태를 기준 커밋으로 남깁니다.

#### 완료 훅

`hooks.after`는 에이전트 세션이 끝난 뒤, 검증 전에 순서대로 실행됩니다. 문자열은
`run`만 적는 축약형이며 `scope`의 기본값은 `workspace`입니다.

| scope       | 실행 위치              | 용도                                |
| ----------- | ---------------------- | ----------------------------------- |
| `workspace` | `$MOHAE_WORKSPACE`      | 검증할 결과를 정리하거나 파일을 생성 |
| `outside`   | 격리된 scratch 디렉터리 | 후처리 출력을 워크스페이스 밖에 생성 |

두 scope 모두 `MOHAE_WORKSPACE`, `MOHAE_TRIAL`, `MOHAE_MODEL`, `MOHAE_EFFORT`와
`agent.env`를 받습니다. `outside`에서도 `$MOHAE_WORKSPACE`로 결과를 읽을 수 있습니다.
`scope`는 권한 경계가 아니라 기본 실행 위치를 고르는 값이므로, `outside` 명령도 해당
환경 변수를 사용해 워크스페이스를 명시적으로 변경할 수 있습니다. 훅 단계는 에이전트 오류나
시간 초과 뒤에도 별도의 제한 시간으로 실행됩니다. 하나라도 실패하면 trial은 실패하지만,
진단을 위해 나머지 훅과 검증은 계속 실행됩니다. 명령, scope, 출력, 종료 코드와 소요 시간은
리포트에 기록됩니다.

#### 검증과 artifact

`verify.commands`는 훅이 끝난 워크스페이스를 scratch 디렉터리에서 검사합니다. 따라서
상대 경로로 만든 검증 파일은 에이전트 결과나 artifact에 섞이지 않습니다. 모든 명령을
실행하며 각 종료 코드가 해당 검사의 판정입니다.

검증 후 `artifacts`와 일치하는 워크스페이스 내부 파일과 디렉터리를 상대 경로 그대로
`report.dir/<trial 이름>-<시각>.artifacts/`에 복사합니다. 심볼릭 링크는 대상을 따라가지
않고 링크 자체를 보존합니다. 일치 항목이 없는 패턴은 리포트에 기록될 뿐 trial을
실패시키지 않습니다. 필수 산출물은 `verify.commands`에서 `test -e` 등으로 검사하세요.

#### 정리와 리포트

통과한 trial의 임시 워크스페이스는 삭제합니다. 실패하거나 검증 명령이 없는 trial은
워크스페이스를 남기고 리포트에 경로를 기록합니다. 검증이 없으면 판정은 `pass`가 아니라
`ungraded`입니다.

`-o`로 고른 형식은 화면에 출력되고, 설정의 `report.formats`와 `-o`가 가리키는 파일 형식은
`report.dir`에 `<trial 이름>-<시각>.<확장자>`로 저장됩니다. 이름이 같은 결과가 같은 초에
생겨도 기존 파일을 덮어쓰지 않습니다. 하나라도 실패하면 종료 코드는 0이 아닙니다.
`--fail-fast`는 첫 실패 뒤 아직 시작하지 않은 trial을 실행하지 않으며, `--concurrency`를
사용해도 리포트 순서는 입력 순서로 유지됩니다. 단, `--show-dialogue` 출력은 병렬 trial
사이에서 섞일 수 있습니다.

MCP 서버는 에이전트 CLI가 읽는 `mcpServers` 형식 그대로 읽습니다. trial 시작 전에
go-sdk로 각 서버에 접속해 도구 목록을 확인하고 리포트에 남기는데, 서버가 뜨지 않은 실행은
"에이전트가 실패한 것"이 아니라 "도구 없이 측정된 것"이기 때문입니다.

`custom-cli` 에이전트는 턴마다 새 프로세스로 실행됩니다. 명령 인자에 `{{prompt}}`가 있으면
그 자리에 프롬프트가 들어가고, 없으면 표준 입력으로 전달됩니다. 표준 출력이 응답입니다.
`MOHAE_WORKSPACE`, `MOHAE_TRIAL`, `MOHAE_MODEL`, `MOHAE_EFFORT`와 `agent.env`는 에이전트
종류와 상관없이 모든 드라이버가 동일하게 전달합니다. mohae가 그 CLI의 플래그를 몰라도 모델과
강도를 넘길 수 있고, `init_script`, 완료 훅, verify 명령도 같은 변수를 읽습니다.

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
| `--target <FIELD>`    | `auto`, `prompts`, `agent-md`, `agent`, `mcp`, `config`       |
| `-n, --repeat <NUM>`  | 반복 횟수 (기본 3)                                          |
| `--metric <TYPE>`     | `success-rate`, `tokens`, `cost`, `duration`                |

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

경고와 실패를 구분합니다. `verify.commands`가 없는 설정은 합격/불합격 판정 없이도 실행은
되므로 경고이고, 경로가 존재하지 않으면 실패입니다.

### `mohae init`

```sh
mohae init                                   # ./mohae.config.yaml
mohae init --all                             # 설정이 참조하는 파일까지 전부
mohae init trials/kvstore --template cli-skill --with-scripts
```

템플릿은 `basic`, `mcp-server`, `cli-skill`, `multi-agent`입니다. 무엇을 테스트 대상으로
두는지만 다르고, 격리·프롬프트·검증이라는 흐름은 모두 같습니다.

설정 파일만 만들면 그 설정이 가리키는 파일들은 아직 없으므로 `mohae verify`가
실패합니다. `--all`은 선택한 템플릿의 설정이 참조하는 파일을 모두 만들어 곧바로
검증이 통과하는 프로젝트를 남깁니다. 개별로는 `--with-scripts`(`init.sh`,
`verify.sh`), `--with-agent-md`, `--with-prompt`(`PROMPT.md`),
`--with-fixture`(`fixture/`), `--with-mcp`(`mcp.json`)로 고를 수 있습니다.

테스트 대상 CLI는 `workspace.init_script`에서 빌드해 `PATH`에 올립니다. 격리된
워크스페이스 안에서 빌드하므로 머신에 설치된 것이 아니라 현재 소스가 평가됩니다.

### `mohae web`

리포트를 읽는 로컬 대시보드입니다(구현 예정). 대화 전문과 워크스페이스 내용이 그대로
담기므로 기본 바인딩은 `127.0.0.1`입니다.

### `mohae report`

과거 리포트를 다른 형식으로 다시 출력하거나, 디렉터리 전체의 토큰·비용·성공률을
집계합니다(구현 예정).

## 구현 상태

| 명령      | 상태                                              |
| --------- | ------------------------------------------------- |
| `init`    | 동작 — 설정과 참조 파일 일체 생성 (`--all`)       |
| `verify`  | 동작 — 경로·스크립트·`AGENTS.md`·컨테이너 런타임 검사 (MCP는 예정) |
| `run`     | 동작 — 실행·훅·검증·artifact·리포트 (`--web`은 예정) |
| `compare` | 인자 검증까지 동작                                |
| `report`  | 인자 검증까지 동작                                |
| `web`     | 미구현                                            |

## 알려진 한계

**`claude-code`에서 워크스페이스는 격리가 아닙니다.** trial은 `workspace.source`를
임시 디렉터리로 복사하고 에이전트를 그곳에서 시작시킬 뿐입니다. `codex`는
샌드박스 정책으로 쓰기를 워크스페이스 안으로 묶지만, `claude-code`에는 아직 그에
대응하는 장치가 없고 벤치마크가 권한 프롬프트에 답할 수 없어 권한 우회 모드로
띄우므로 호스트의 아무 경로나 읽고 쓸 수 있습니다. 실제로 프롬프트가 경로를 명시하지
않자 에이전트가 워크스페이스 밖에 결과물을 만들어 `when` 조건과 `verify`가 함께
어긋나는 일이 관측됐습니다. 두 에이전트가 같은 규칙에서 측정되지 않는다는 뜻이기도
합니다. [`container.scope: full`](#컨테이너-격리)이 이를 해소합니다 — 그 경우 두
에이전트 모두 컨테이너가 경계입니다.

**실패한 trial의 워크스페이스는 남습니다.** 무엇이 일어났는지 볼 방법이 그것뿐이기
때문입니다. 24시간이 지난 것은 다음 `run`이 정리하며, 정리한 개수를 출력합니다.
컨테이너는 남기지 않습니다 — 워크스페이스를 보존하는 경우에도 trial이 끝나면 제거하고,
mohae가 강제 종료돼 남은 컨테이너는 다음 `run`이 회수합니다.

## 개발

Go 1.26.3 이상이 필요합니다.

```sh
cd cli/mohae
go test ./...
go build -o mohae .
```

소스는 의존 방향에 따라 나뉩니다.

```text
main.go → cmd/ → internal/config, runner, report, scaffold
                    runner → agent, driver, process
                    driver → claude, codex
```

`cmd/{command}.go`는 한 명령의 플래그와 액션을 함께 소유합니다. 설정 해석은
`internal/config`, trial 실행과 워크스페이스는 `internal/runner`, 결과 출력은
`internal/report`, `init` 템플릿은 `internal/scaffold`가 담당합니다.
