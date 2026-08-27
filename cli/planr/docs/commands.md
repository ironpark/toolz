# 명령어 가이드

[README로 돌아가기](../README.md) · [설정](configuration.md) · [문서 규격](document-format.md)

## 명령 요약

| 명령 | 설명 |
| --- | --- |
| `planr new <plan-name>` | 새 plan 초안 파일을 생성합니다 |
| `planr new <plan-name>#<phase-name>` | 열린 plan에 추가할 phase 초안을 생성합니다 |
| `planr edit <plan-name>#<phase-number>` | 기존 phase를 편집할 수 있도록 checkout합니다 |
| `planr edit <plan-name> --section goals\|context\|plan` | plan section을 checkout합니다 |
| `planr apply <file> [--dry-run] [--json]` | 문서를 검증하고 등록하거나 checkout을 적용합니다 |
| `planr apply --stdin [--dry-run] [--json]` | 표준 입력의 문서를 검증하고 적용합니다 |
| `planr status [plan-name] [--json]` | 남은 phase와 대기 중인 의존성을 자세히 출력합니다 |
| `planr show <plan-name> [phase-number] [--json]` | 현재 또는 지정한 phase를 출력합니다 |
| `planr show <plan-name> --section goals\|context\|plan` | plan section 문서를 출력합니다 |
| `planr show <plan-name> --all --json` | plan 전체 문서를 하나의 JSON으로 출력합니다 |
| `planr overview [plan-name] [--json]` | 모든 plan의 진행률을 한 줄씩 요약합니다 |
| `planr phase set <plan-name> <number> --status <status>` | phase 상태를 직접 지정합니다 |
| `planr phase start\|done\|reset <plan-name> <number>` | phase 상태 변경 단축 명령입니다 |
| `planr phase rm <plan-name> <number> [--force]` | phase 문서와 checklist 항목을 삭제합니다 |
| `planr archive <plan-name>` | 완료된 plan을 마지막 `plans_dirs` 경로로 옮깁니다 |
| `planr notes [plan-name] [--json]` | 커밋에 연결된 phase·plan 이벤트를 조회합니다 |
| `planr config [--json]` | 실제 적용된 설정을 출력합니다 |
| `planr doctor [--fix] [--json]` | 설정·저장소·문서 정합성을 진단합니다 |
| `planr schema [--json]` | 문서의 필수 구조와 적용 규칙을 출력합니다 |
| `planr completion <shell>` | 셸 자동 완성 스크립트를 출력합니다 |
| `planr --version` | 설치된 버전을 출력합니다 |

plan 이름을 받는 명령은 `checkout-v2`와 `00-checkout-v2`를 모두 인식합니다. 전역
`--no-hooks`는 해당 호출의 `before`·`after` 훅을 건너뜁니다.

## 생성과 적용

```sh
# 새 plan
planr new platform-refresh --description "platform plan refresh"

# 선행 plan 또는 특정 phase 지정: 옵션 반복과 쉼표 구분을 모두 지원합니다.
planr new checkout-v2 --description "checkout flow refresh" \
  --depends-on platform-refresh --depends-on api-foundation#2

planr apply checkout-v2.md
```

| 옵션 | 명령 | 설명 |
| --- | --- | --- |
| `--description` | `new` | 필수인 200자 이내 설명입니다. plan 이름 뒤의 두 번째 인자로도 지정할 수 있습니다 |
| `--depends-on` | `new` | 선행 plan 또는 `plan-name#phase-number`를 반복해서 지정합니다 |
| `--output` | `new`, `edit` | 출력 경로입니다. `edit`의 기본값은 저장소의 `.planr/` scratch 디렉터리입니다 |
| `--json` | `new`, `edit`, `apply` | 템플릿, checkout 또는 적용 결과를 JSON으로 출력합니다 |
| `--stdin` | `apply` | 파일 대신 표준 입력에서 문서를 읽습니다 |
| `--dry-run` | `apply` | 검증과 변경 결과만 보고 파일을 쓰지 않습니다 |

`new`는 작성 전 문서를 만들고 `apply`는 문서 frontmatter를 읽어 새 plan 등록, phase 추가,
기존 문서 편집 중 무엇인지 결정합니다.

```sh
# 파일 기반 작업
planr new checkout-v2 --description "checkout flow refresh"
planr apply checkout-v2.md

# 에이전트·스크립트에서 파일 없이 작업
planr new checkout-v2 --description "checkout flow refresh" --json
planr apply --stdin --json

# 기존 phase나 section 편집
planr edit checkout-v2#2 --json
planr edit checkout-v2 --section goals --json
planr apply --stdin --json
```

`edit` checkout에는 `planr_edit`, `planr_target`, `planr_base`가 들어갑니다. `apply`는
checkout 시점의 SHA-256과 현재 문서를 비교해 동시 수정을 막습니다. 대상이 달라졌다면
다시 `edit`해야 합니다.

`edit --section plan`의 phase checklist는 보호되는 파생 영역입니다. marker와 그 사이를
수정하지 마세요. `apply`는 디스크의 checklist와 `plan_status`를 유지합니다. 파일 기반
checkout을 쓴다면 저장소 `.gitignore`에 `/.planr/`를 추가하는 것이 좋습니다.

## 조회

```sh
planr status
planr status checkout-v2

planr show checkout-v2       # 첫 번째 미완료 phase
planr show checkout-v2 2     # 지정한 phase
planr show checkout-v2 --section goals

planr overview
planr notes checkout-v2
```

- `status`는 진행 중 plan의 남은 phase와 `wait` 의존성을 보여 줍니다. 완료 plan은 기본적으로
  숨기지만 이름을 지정하면 표시합니다.
- `overview`는 완료 plan을 포함한 상태, `완료 phase/전체 phase`, 다음 phase와 대기를
  간단히 보여 줍니다.
- `show`는 번호를 생략하면 첫 번째 미완료 phase를 선택합니다. `--all`은 `--json`과 함께
  plan 전체 문서를 반환합니다.
- `notes`는 시작·완료 이벤트가 기록된 커밋을 보여 줍니다.

### JSON 출력

`status`, `overview`, `notes`의 최상위 키는 각각 `plans`, `plans`, `notes`입니다. 결과가
없어도 배열은 `null`이 아니라 빈 배열입니다.

- `status.plans[]`: `name`, `directory`, `status`, `done_phases`, `total_phases`,
  `remaining`, `wait`
- `overview.plans[]`: `name`, `directory`, `status`, `done_phases`, `total_phases`,
  `next_phase`, `wait`
- `show`: `name`, `directory`, `phase_number`, `slug`, `title`, `status`, `planned_work`,
  `done_when`, `depends_on`, `file`
- `notes.notes[]`: `completed_at`, `plan`, `event`, `phase`, `commit`, `short_commit`, `subject`

`planr apply --json`은 검증 실패도 `rule`, `section`, `phase`, `line`, `detail` 필드가 있는
구조화된 오류로 반환합니다.

## 설정 확인과 진단

```sh
planr config
planr config --json

planr doctor
planr doctor --json
planr doctor --fix
```

`config`는 선택된 설정 파일, 저장소 루트, 감지한 에이전트와 최종 설정값을 출력합니다.
설정 파일이 없으면 기본값을 명시합니다.

`doctor`는 설정 파싱, 저장 경로, Git 저장소, PLAN·phase frontmatter, 의존성, 파일명과
checklist 링크·제목·상태의 일치를 검사합니다. 기본 동작은 읽기 전용이며 문제가 있으면
non-zero로 종료합니다. `--fix`는 유효한 phase 파일을 기준으로 `PLAN.md` checklist만
복구하며 frontmatter나 의존성 오류는 자동 수정하지 않습니다.

## phase 관리

```sh
planr phase start checkout-v2 1
planr phase done checkout-v2 1
planr phase reset checkout-v2 1
planr phase set checkout-v2 1 --status conditional
```

| 명령 | 결과 상태 |
| --- | --- |
| `phase start` | `in-progress` |
| `phase done` | `done` |
| `phase reset` | `planned` |

### phase 추가와 삭제

```sh
planr new checkout-v2#Cache-Warmup --json
# 반환된 template을 채운 뒤:
planr apply --stdin

planr phase rm checkout-v2 2
planr phase rm checkout-v2 2 --force
```

추가 초안의 Planned Work와 Done When은 필수입니다. `depends_on`에는 같은 plan의 기존
phase 번호나 slug를 쓰며, status는 `planned` 또는 `conditional`만 허용합니다.
`conditional`에는 `entry_condition`이 필요합니다. 번호는 기존 최댓값 다음으로 자동
배정됩니다.

삭제 시 남은 phase 번호, 파일명, 의존성과 Git note는 다시 번호를 매기지 않습니다.
다른 phase가 삭제 대상을 참조하면 거부되며 의존성을 직접 정리할 때만 `--force`를 씁니다.

### 상태 변경 전 검사

`phase start`는 phase와 plan 의존성이 모두 완료되었는지 검사합니다. `phase done`은 같은
검사와 함께 plan 문서·`.planr.yaml`을 제외한 미커밋 소스가 없는지도 확인합니다.

```text
$ planr phase start checkout-v2 1
cannot set 00-checkout-v2 phase 01 to in-progress while its dependencies are unfinished:
  - phase 00 "API Contract" (planned)
finish them first or use --force
```

두 검사는 `--force`로 우회할 수 있습니다. 의도적으로 순서를 벗어나거나 미커밋 변경을
포함해야 할 때만 사용하세요. 상태를 되돌리는 `phase reset`에는 이 검사가 적용되지 않습니다.

## 완료 plan 보관

```sh
planr archive checkout-v2
```

`archive`는 첫 번째 `plans_dirs`에 있는 완료 plan 디렉터리를 마지막 경로로 옮깁니다.
번호는 유지되고 이후 plan 번호도 모든 경로의 최댓값 다음으로 이어집니다. 저장 경로가
하나뿐이거나, plan이 완료되지 않았거나, 이미 마지막 경로에 있으면 보관할 수 없습니다.

## 동시 실행과 훅 건너뛰기

상태 변경 명령은 같은 plan의 `.planr.lock`을 획득합니다. 새 plan 등록은 첫 번째 저장
경로의 lock으로 번호 배정과 등록을 직렬화합니다. 조회 명령은 lock을 사용하지 않습니다.

일시적으로 훅을 건너뛰려면 전역 옵션을 사용합니다.

```sh
planr phase done checkout-v2 2 --no-hooks
```

## 셸 자동 완성

```sh
# bash
echo 'source <(planr completion bash)' >> ~/.bashrc

# zsh
echo 'source <(planr completion zsh)' >> ~/.zshrc

# fish
mkdir -p ~/.config/fish/completions
planr completion fish > ~/.config/fish/completions/planr.fish

# PowerShell
planr completion pwsh > planr_completion.ps1
```

새 셸을 시작하면 plan 이름을 받는 명령에서 모든 `plans_dirs`의 plan도 동적으로
완성됩니다.
