# chatctl

ChatGPT, Gemini, Claude 의 **웹 세션** 대화 목록을 관리하는 Go CLI 입니다.
공식 API 대신 chromedp 로 실제 브라우저 세션을 사용하므로, 웹에 로그인한 계정의 대화를 그대로 다룹니다.

## 설치

```sh
go install github.com/ironpark/toolz/chatctl@latest
```

로컬 Chrome / Chromium 이 설치되어 있어야 합니다.

## 사용법

```sh
# 지원 서비스 확인
chatctl providers

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

계정을 여러 개 쓸 때는 프로필을 나누면 됩니다.

## 한계

각 서비스의 사이드바 DOM 구조에 의존해 대화 목록을 수집합니다.
서비스 UI 가 바뀌면 `internal/provider/provider.go` 의 셀렉터 상수를 갱신해야 합니다.
