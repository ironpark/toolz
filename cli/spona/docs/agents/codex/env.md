# OpenAI Codex CLI (`codex`) — 환경변수

> [README](README.md) · [../](../README.md)

공식 설정 레퍼런스는 환경변수를 별도 표로 열거하지 않고, 대부분을 `config.toml`과
`model_providers.<id>.env_key` 같은 **간접 참조**로 다룹니다. 아래는 공식 문서에서
확인되는 항목과 실행 파일에서 확인되는 항목을 구분한 것입니다.

## 공식 문서에서 확인

| 변수 | 설명 |
| --- | --- |
| `CODEX_HOME` | Codex 설정 디렉터리 루트 (**프리셋 격리의 핵심**) |
| `OPENAI_API_KEY` | 기본 `openai` 프로바이더의 `env_key` 대상 |
| `model_providers.<id>.env_key`가 가리키는 임의 변수 | 프로바이더별 API 키 |
| `mcp_servers.<id>.env` / `.env_vars`, `.bearer_token_env_var` | MCP 서버에 전달·화이트리스트되는 변수 |
| `--remote-auth-token-env`가 가리키는 변수 | 원격 app server 베어러 토큰 |

## 실행 파일 문자열에서 확인 (문서화되지 않음, 참고용)

`CODEX_API_KEY`, `CODEX_ACCESS_TOKEN`, `CODEX_AUTH`, `CODEX_URL`,
`CODEX_CA_CERTIFICATE`, `CODEX_SQLITE_HOME`, `CODEX_NON_INTERACTIVE`,
`CODEX_SESSION_ID`, `CODEX_THREAD_ID`, `CODEX_PERMISSION_PROFILE`,
`CODEX_APPLY_GIT_CFG`, `CODEX_APPLY_PATCH_PRESERVE_LINE_ENDINGS`,
`CODEX_GITHUB_PERSONAL_ACCESS_TOKEN`, `CODEX_MCP_PROTOCOL_VERSION`,
`CODEX_MANAGED_PACKAGE_ROOT`, `CODEX_INTERNAL_ORIGINATOR_OVERRIDE`,
`RUST_LOG`, `NO_COLOR`.

> [!NOTE]
> Codex는 설정 대부분을 `config.toml` + `-c` 오버라이드로 표현하도록 설계되어 있습니다.
> spona도 환경변수는 `CODEX_HOME`과 자격증명 위주로만 다루고, 나머지 설정은 `-c` 또는
> 프로필 파일로 전달하는 편이 공식 모델과 일치합니다.
