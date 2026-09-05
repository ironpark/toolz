# OpenAI Codex CLI (`codex`) — 설정 레이어와 `config.toml`

> [README](README.md) · [../](../README.md)

## 설정 레이어와 우선순위

공식 문서 기준, **높은 순**:

1. CLI 플래그 및 `-c/--config` 오버라이드
2. 프로젝트 설정 `<project>/.codex/config.toml` — 가장 가까운 디렉터리 우선, **신뢰된 프로젝트만**
3. 프로필 파일 `$CODEX_HOME/<name>.config.toml` (`--profile`로 선택)
4. 사용자 설정 `$CODEX_HOME/config.toml` (기본 `~/.codex/config.toml`)
5. 시스템 설정 `/etc/codex/config.toml` (Unix)
6. 내장 기본값

여기에 조직이 `requirements.toml`로 제약을 강제할 수 있습니다.

두 가지 중요한 제약:

- **프로젝트 설정이 덮어쓸 수 없는 영역**: 머신 로컬 프로바이더, 인증, 호스트 소유 메타데이터,
  알림, 프로필 선택, 텔레메트리 키.
- **신뢰(trust) 모델**: 프로젝트를 untrusted로 표시하면 Codex는 프로젝트 스코프 `.codex/`
  레이어 전체(프로젝트 로컬 config, 훅, rules)를 건너뜁니다.
  `projects.<path>.trust_level` 키로 `trusted` / `untrusted` 를 지정합니다.

> [!IMPORTANT]
> spona가 프로젝트 디렉터리에 `.codex/config.toml`을 써 두는 방식으로 프리셋을 적용한다면,
> 해당 프로젝트가 신뢰되지 않은 상태면 **조용히 무시**됩니다. `CODEX_HOME` 격리 또는
> `-c` 오버라이드 방식이 더 결정적입니다.

### `$CODEX_HOME` 디렉터리 구조 (기본 `~/.codex`)

| 경로 | 역할 |
| --- | --- |
| `config.toml` | 사용자 설정 |
| `<name>.config.toml` | `--profile <name>` 프로필 레이어 |
| `auth.json` | 인증 자격증명 (`cli_auth_credentials_store`로 keyring 사용 가능) |
| `hooks.json` | 훅 정의 |
| `history.jsonl` | 프롬프트 히스토리 |
| `sessions/`, `archived_sessions/` | 세션 기록 |
| `plugins/`, `packages/` | 플러그인·패키지 |
| `log/` | 로그 (`log_dir`로 변경 가능) |
| `AGENTS.md` | 사용자 수준 에이전트 지침 |

프로젝트 지침은 `AGENTS.md`이며, `model_instructions_file`로 대체하거나
`project_doc_fallback_filenames` / `project_doc_max_bytes` / `project_root_markers`로
탐색 동작을 바꿀 수 있습니다.

## `config.toml` 주요 키

전체 목록은 [config-reference](https://learn.chatgpt.com/docs/config-file/config-reference) 참고.
spona 프리셋 스키마에 직접 관계되는 것만 추립니다.

### 코어

| 키 | 설명 |
| --- | --- |
| `model` | 모델 식별자 |
| `model_provider` | `model_providers` 테이블의 프로바이더 ID (기본 `openai`) |
| `model_context_window` | 컨텍스트 윈도 토큰 수 |
| `model_reasoning_effort` | 추론 강도 |
| `model_reasoning_summary` | 추론 요약 상세도 |
| `model_verbosity` | 응답 장황도 |
| `model_instructions_file` | `AGENTS.md` 대신 쓸 지침 파일 |
| `personality` | `none`, `friendly`, `pragmatic` |
| `developer_instructions` | 추가 개발자 지침 |
| `review_model`, `plan_mode_reasoning_effort`, `service_tier` | 용도별 모델·티어 오버라이드 |

### 프로바이더 / 인증

| 키 | 설명 |
| --- | --- |
| `model_providers.<id>.base_url` | API 엔드포인트 |
| `model_providers.<id>.env_key` | **API 키를 담을 환경변수 이름** |
| `model_providers.<id>.wire_api` | 프로토콜 (기본 `responses`) |
| `model_providers.<id>.http_headers` / `.env_http_headers` / `.query_params` | 헤더·쿼리 주입 |
| `model_providers.<id>.auth.command` / `.auth.refresh_interval_ms` | 베어러 토큰 생성 명령과 갱신 주기 |
| `openai_base_url`, `chatgpt_base_url` | 기본 엔드포인트 재지정 |
| `forced_login_method` (`chatgpt`\|`api`), `forced_chatgpt_workspace_id` | 로그인 제한 |
| `cli_auth_credentials_store` | `file`, `keyring`, `auto` |
| `oss_provider` | 기본 로컬 프로바이더 (`lmstudio`\|`ollama`) |

### 샌드박스 / 승인 / 권한 프로필

| 키 | 설명 |
| --- | --- |
| `sandbox_mode` | `read-only`, `workspace-write`, `danger-full-access` |
| `approval_policy` | `untrusted`, `on-request`, `never` 또는 granular 테이블 |
| `approval_policy.granular.*` | `sandbox_approval`, `rules`, `mcp_elicitations`, `request_permissions`, `skill_approval` 개별 허용 |
| `approvals_reviewer` | `user` 또는 `auto_review` |
| `sandbox_workspace_write.writable_roots` | 추가 쓰기 허용 루트 |
| `sandbox_workspace_write.network_access` | workspace-write에서 아웃바운드 네트워크 허용 |
| `sandbox_workspace_write.exclude_slash_tmp` / `.exclude_tmpdir_env_var` | `/tmp`·`$TMPDIR` 제외 |
| `default_permissions` | 이름 있는 권한 프로필. 내장: `:read-only`, `:workspace`, `:danger-full-access` |
| `permissions.<name>.*` | 커스텀 권한 프로필: `extends`, `filesystem.<path>`(`read`/`write`/`deny`), `workspace_roots`, `network.enabled` / `.domains` / `.unix_sockets` / `.mode` / `.proxy_url` |

**`permissions.<name>` 프로필은 spona의 "권한 프리셋"에 그대로 대응됩니다.**
프리셋을 프로필로 내보내고 `default_permissions`로 선택하는 방식이 자연스럽습니다.

### 셸 환경 (환경변수 주입 시 필수 확인)

| 키 | 설명 |
| --- | --- |
| `shell_environment_policy.inherit` | 상속 기준: `all`, `core`, `none` |
| `shell_environment_policy.set` | 명시적으로 설정할 환경변수 맵 |
| `shell_environment_policy.filters` | 포함/제외 패턴 |
| `shell_environment_policy.ignore_default_excludes` | `KEY`/`SECRET`/`TOKEN` 포함 변수도 유지 |
| `shell_environment_policy.experimental_use_profile` | 사용자 셸 프로필 사용 |
| `allow_login_shell` | 로그인 셸 시맨틱 허용 (기본 true) |

> [!WARNING]
> 기본적으로 이름에 `KEY`/`SECRET`/`TOKEN`이 들어간 변수는 제외됩니다. spona가 프리셋의
> 자격증명을 에이전트 셸까지 전달하려면 `shell_environment_policy.set`에 명시하거나
> `ignore_default_excludes`를 켜야 합니다.

### MCP

| 키 | 설명 |
| --- | --- |
| `mcp_servers.<id>.command` / `.args` / `.cwd` / `.env` / `.env_vars` | stdio 서버 실행 정의 |
| `mcp_servers.<id>.url` / `.bearer_token_env_var` / `.auth` | HTTP 서버와 인증(`oauth`\|`chatgpt`) |
| `mcp_servers.<id>.enabled` / `.required` | 활성화 / 없으면 시작 실패 |
| `mcp_servers.<id>.enabled_tools` / `.disabled_tools` | 도구 allow/deny |
| `mcp_servers.<id>.tools.<tool>.approval_mode` / `.output_token_limit` | 도구별 승인·출력 제한 |
| `mcp_servers.<id>.startup_timeout_sec` / `.tool_timeout_sec` | 타임아웃 |
| `mcp_oauth_credentials_store`, `mcp_oauth_callback_url`, `mcp_oauth_callback_port` | 전역 OAuth 설정 |

### 멀티 에이전트 (spona와 직접 겹치는 영역)

| 키 | 설명 |
| --- | --- |
| `agents.enabled` | 멀티 에이전트 도구 활성화 (기본 true) |
| `agents.default_subagent_model` | spawn 되는 에이전트 기본 모델 |
| `agents.default_subagent_reasoning_effort` | 기본 추론 강도 |
| `agents.max_concurrent_threads_per_session` | 세션당 최대 동시 스레드 |
| `agents.<name>.description` | 역할 선택·spawn 기준 설명 |
| `agents.<name>.config_file` | **역할별 TOML 설정 레이어 경로** |

> [!TIP]
> `agents.<name>.config_file` 은 "이름 붙은 역할 = 설정 레이어 파일"이라는 구조로,
> spona의 프리셋 개념과 사실상 동일합니다. Codex 대상 프리셋은 이 형태로 내보내면
> Codex 자체 멀티 에이전트 기능과도 호환됩니다.

### 도구 / 피처 / 훅

| 키 | 설명 |
| --- | --- |
| `web_search` | `disabled`, `cached`, `indexed`, `live` |
| `tools.web_search`, `tools.view_image` | 개별 도구 토글 |
| `tool_output_token_limit` | 도구 출력 토큰 예산 |
| `features.shell_tool`, `features.unified_exec`, `features.shell_snapshot` | 셸 도구 동작 |
| `features.hooks` | 라이프사이클 훅 (기본 false) |
| `features.multi_agent`, `features.memories`, `features.goals`, `features.apps`, `features.remote_plugin`, `features.fast_mode` | 주요 기능 토글 |
| `features.network_proxy.*` | 샌드박스 명령용 프록시(`enabled`, `domains`, `proxy_url`, `socks_url` 등) |
| `hooks.<Event>` | `PreToolUse`, `PostToolUse`, `SessionStart` 등 이벤트별 matcher 그룹 |
| `skills.config[]` (`path`, `enabled`), `skills.max_context_tokens` | 스킬 활성화·컨텍스트 예산 |
| `plugins.<plugin>.mcp_servers.<server>.*` | 플러그인 제공 MCP 서버 제어 |

### 히스토리 / 저장소 / 관측

| 키 | 설명 |
| --- | --- |
| `history.persistence` (`save-all`\|`none`), `history.max_bytes` | 트랜스크립트 보관 |
| `model_auto_compact_token_limit`(+`_scope`) | 자동 컴팩션 임계 |
| `sqlite_home`, `log_dir` | 상태 DB·로그 위치 |
| `analytics.enabled`, `otel.*`, `notify` | 텔레메트리와 알림 명령 |
| `projects.<path>.trust_level` | 프로젝트 신뢰 수준 |
