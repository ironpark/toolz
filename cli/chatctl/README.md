# chatctl

ChatGPT, Gemini, Claude 의 **웹 세션** 대화 목록을 관리하는 Go CLI 입니다.
공식 API 대신 chromedp 로 실제 브라우저 세션을 사용하므로, 웹에 로그인한 계정의 대화를 그대로 다룹니다.

## 설치

```sh
go install github.com/ironpark/toolz/cli/chatctl@latest
```

로컬 Chrome / Chromium 이 설치되어 있어야 합니다.

## 엔진

| 엔진 | 설명 |
| --- | --- |
| `moli` | [moli](https://github.com/lexmount/moli) 의 `moli serve` 를 띄우고 CDP 로 접속합니다. 헤드리스 전용이며 Chrome 을 실행하지 않습니다. |
| `chrome` | 로컬 Chrome / Chromium 을 직접 실행합니다. 창이 필요한 작업에 사용합니다. |
| `auto` | 기본값. 헤드리스 작업에서 `moli` 가 설치되어 있으면 `moli`, 없으면 `chrome` 을 씁니다. |

`--engine` 또는 `CHATCTL_ENGINE` 로 지정합니다.
`login` 과 `open`, `list --show` 는 사람이 화면을 봐야 하므로 항상 `chrome` 으로 동작합니다.

moli 는 Chrome 프로필을 공유하지 못하므로, `login` 이 끝나면 쿠키를 프로필의 `cookies.json` 으로 내보내고
moli 세션을 시작할 때 CDP 로 다시 주입합니다. 즉 로그인은 Chrome 으로 한 번, 이후 조회는 moli 로 처리됩니다.

## 사용법

```sh
# 지원 서비스 확인
chatctl providers

# 엔진 / 프로필 / 쿠키 상태 점검
chatctl doctor

# 로그인 (브라우저 창이 열립니다. 로그인 후 Enter)
chatctl login chatgpt

# 대화 목록 (인자 생략 시 전체 서비스)
chatctl list
chatctl list claude --limit 20
chatctl list --json

# 특정 대화 열기
chatctl open chatgpt c/<대화ID>
```

## 프로필

로그인 세션은 프로필 단위의 Chrome 사용자 데이터 디렉터리에 저장됩니다.

- 위치: `<UserConfigDir>/chatctl/profiles/<프로필>`
- 지정: `--profile <이름>` 또는 `CHATCTL_PROFILE` 환경 변수
- 구성: Chrome 사용자 데이터, moli 프로필(`moli/`), 내보낸 쿠키(`cookies.json`, 권한 `0600`)

계정을 여러 개 쓸 때는 프로필을 나누면 됩니다.

### 기존 Chrome 프로필 사용

이미 로그인해 둔 Chrome 프로필이 있다면 별도 로그인 없이 그대로 쓸 수 있습니다.
chatctl 프로필에 저장된 쿠키가 없으면 OS 기본 위치(예: macOS
`~/Library/Application Support/Google/Chrome`)의 Chrome/Chromium 프로필을 자동
탐색해 사용하므로, 보통은 아무 플래그 없이 `chatctl list` 만으로 동작합니다.

경로를 직접 지정할 수도 있습니다.

```sh
chatctl --user-data-dir "$HOME/Library/Application Support/Google/Chrome" list
```

- 지정: `--user-data-dir <경로>` (`-d`) 또는 `CHATCTL_USER_DATA_DIR` 환경 변수
- 우선순위: `--user-data-dir` → 쿠키가 저장된 chatctl 프로필 → 자동 탐색된 Chrome 프로필
- `login` 은 chatctl 프로필에 세션을 만드는 명령이므로 자동 탐색을 하지 않습니다
- 이 경우 엔진은 항상 chrome 이며, moli 는 사용할 수 없습니다
- Chrome 은 같은 사용자 데이터 디렉터리를 두 프로세스가 동시에 열 수 없으므로,
  해당 프로필을 쓰는 Chrome 을 먼저 종료해야 합니다

## 지연 로딩

세 서비스 모두 대화 목록을 한 번에 내려주지 않습니다.

| 서비스 | 방식 | 대응 |
| --- | --- | --- |
| ChatGPT | 사이드바를 아래로 스크롤해야 다음 묶음 로드 | 스크롤 컨테이너를 바닥까지 반복 스크롤 |
| Claude | 사이드바는 최근 항목만, "모든 대화 보기" 가 전체 목록 | `/recents` 를 직접 열고 표를 스크롤 |
| Gemini | "더보기" 버튼 클릭 | 버튼이 사라질 때까지 반복 클릭 |

`list` 는 개수가 두 번 연속 늘지 않으면 끝으로 판단합니다.
`--limit N` 을 주면 N개가 모이는 즉시 멈추므로 훨씬 빠릅니다.

## 한계

각 서비스의 사이드바 DOM 구조에 의존해 대화 목록을 수집합니다.
서비스 UI 가 바뀌면 `internal/provider/provider.go` 의 셀렉터 상수를 갱신해야 합니다.

ChatGPT 의 프로젝트와 Claude 의 고정된 프로젝트는 대화 링크가 아니라서 목록에 잡히지 않습니다.
