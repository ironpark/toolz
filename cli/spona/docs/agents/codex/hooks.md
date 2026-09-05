# OpenAI Codex CLI (`codex`) — 훅

> [README](README.md) · [../](../README.md)

출처: <https://learn.chatgpt.com/docs/config-file/config-advanced>,
<https://github.com/openai/codex/blob/main/docs/config.md>

라이프사이클 훅은 `$CODEX_HOME/hooks.json` 또는 `config.toml`의 인라인 `[hooks]` 테이블로
정의하며, `PreToolUse` 같은 이벤트에 반응합니다.
`features.hooks` 로 활성화하며 **기본값은 false**입니다.

로컬 `config.toml`에서 확인되는 이벤트 키:
`session_start`, `user_prompt_submit`, `pre_tool_use`, `post_tool_use`,
`subagent_start`, `subagent_stop`, `permission_request`, `stop`.

`config.toml`의 `[hooks.state]` 테이블은 훅 **신뢰 상태**를 기록합니다
(`"<hooks.json 경로>:<event>:<idx>:<idx>"` 키 형태).
`codex --dangerously-bypass-hook-trust` 는 이 신뢰 등록을 건너뛰고 실행합니다.

관리자는 `requirements.toml`에 최상위 `allow_managed_hooks_only = true` 를 설정해
user·project·session 훅 설정을 무시하고 관리형 훅만 허용할 수 있습니다.
**이 설정은 `requirements.toml`에서만 유효하며 `config.toml`에 넣으면 동작하지 않습니다.**

훅 외에 `notify` 설정으로 외부 프로그램에 JSON 알림을 보낼 수도 있습니다
(데스크톱 토스트, 웹훅 등).
