# Claude Code (`claude`) — 환경변수

> [README](README.md) · [../](../README.md)

## 인증

| 변수 | 설명 |
| --- | --- |
| `ANTHROPIC_API_KEY` | `X-Api-Key` 헤더로 전송. 설정 시 구독 인증을 덮어씀 |
| `ANTHROPIC_AUTH_TOKEN` | 커스텀 `Authorization` 값(`Bearer` 접두) |
| `ANTHROPIC_PROFILE` | 인증에 사용할 Anthropic 프로필 이름 |
| `ANTHROPIC_ORGANIZATION_ID`, `ANTHROPIC_WORKSPACE_ID`, `ANTHROPIC_FEDERATION_RULE_ID` | Workload Identity Federation |

## 프로바이더

| 변수 | 설명 |
| --- | --- |
| `ANTHROPIC_BASE_URL` | API 엔드포인트 재지정(프록시/게이트웨이) |
| `ANTHROPIC_BETAS` | `anthropic-beta` 헤더 값 콤마 목록 |
| `ANTHROPIC_CUSTOM_HEADERS` | 커스텀 헤더 (`Name: Value`) |
| `CLAUDE_CODE_USE_VERTEX` | Google Cloud Agent Platform 사용 |
| `ANTHROPIC_VERTEX_BASE_URL`, `ANTHROPIC_VERTEX_PROJECT_ID` | Vertex 설정 |
| `ANTHROPIC_BEDROCK_BASE_URL`, `ANTHROPIC_BEDROCK_REGION_PREFIX`, `ANTHROPIC_BEDROCK_SERVICE_TIER`, `AWS_BEARER_TOKEN_BEDROCK` | Bedrock 설정 |
| `ANTHROPIC_AWS_API_KEY`, `ANTHROPIC_AWS_BASE_URL`, `ANTHROPIC_AWS_WORKSPACE_ID` | Claude Platform on AWS |
| `ANTHROPIC_FOUNDRY_API_KEY`, `ANTHROPIC_FOUNDRY_AUTH_TOKEN`, `ANTHROPIC_FOUNDRY_BASE_URL`, `ANTHROPIC_FOUNDRY_RESOURCE` | Microsoft Foundry |

## 모델

| 변수 | 설명 |
| --- | --- |
| `ANTHROPIC_MODEL` | 사용할 모델 설정 이름. **파일의 `model` 키보다 우선** |
| `ANTHROPIC_DEFAULT_MODEL` | 새 세션의 시작 모델. **파일이 `model`을 설정하지 않은 경우에만** 적용 (v2.1.236+) |
| `ANTHROPIC_DEFAULT_OPUS_MODEL` / `_SONNET_` / `_HAIKU_` / `_FABLE_` | 각 별칭이 해석될 모델 ID |
| `ANTHROPIC_CUSTOM_MODEL_OPTION`(+`_NAME`/`_DESCRIPTION`/`_SUPPORTED_CAPABILITIES`) | `/model` picker에 커스텀 모델 추가 |
| `ANTHROPIC_SMALL_FAST_MODEL` | **[deprecated]** 백그라운드용 Haiku급 모델 |

## 네트워크 / 타임아웃

| 변수 | 설명 |
| --- | --- |
| `API_TIMEOUT_MS` | API 요청 타임아웃 (기본 600000) |
| `API_FORCE_IDLE_TIMEOUT` | 5분 body idle 타임아웃 재정의 (`0` 끄기 / `1` 유지) |
| `BASH_DEFAULT_TIMEOUT_MS` | Bash 기본 타임아웃 (기본 120000) |
| `BASH_MAX_TIMEOUT_MS` | 모델이 지정 가능한 최대 Bash 타임아웃 (기본 600000) |
| `BASH_MAX_OUTPUT_LENGTH` | Bash 출력 최대 문자 수 (기본 30000, 최대 150000) |
| `CLAUDE_ASYNC_AGENT_STALL_TIMEOUT_MS` | 서브에이전트 stall 타임아웃 (기본 600000) |
| `CLAUDE_STREAM_IDLE_TIMEOUT_MS`, `CLAUDE_BYTE_STREAM_IDLE_TIMEOUT_MS` | 스트리밍 idle 감시 |

## 경로 / 동작

| 변수 | 설명 |
| --- | --- |
| `CLAUDE_CONFIG_DIR` | `~/.claude`의 위치를 변경. 설정·세션 히스토리·플러그인이 여기 저장됨 (**프리셋 격리의 핵심**) |
| `CLAUDECODE` | Claude Code가 spawn 한 하위 프로세스에 `1`로 설정됨 |
| `CLAUDE_CODE_CHILD_SESSION` | 도구/훅이 직접 spawn 한 프로세스인지 표시 |
| `CLAUDE_BASH_MAINTAIN_PROJECT_WORKING_DIR` | 각 Bash 명령 후 원래 디렉터리로 복귀 |
| `CLAUDE_AUTO_BACKGROUND_TASKS` | 장기 작업 자동 백그라운드화 강제 |
| `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` | 비필수 트래픽 차단 |
| `CLAUDE_CODE_DISABLE_1M_CONTEXT` | 1M 컨텍스트 모델 비활성화 |
| `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE` | 자동 컴팩트 발동 비율(1–100) |
| `CLAUDE_AFK_TIMEOUT_MS`, `CLAUDE_AFK_COUNTDOWN_MS` | 미응답 다이얼로그 자동 진행 |
| `DISABLE_TELEMETRY`, `DISABLE_ERROR_REPORTING` | 텔레메트리/에러 리포팅 비활성화 |
| `CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS` | 내장 서브에이전트 타입 비활성화 (SDK 전용) |

## 환경변수 ↔ 설정 키 우선순위

- `ANTHROPIC_MODEL`, `CLAUDE_CODE_AUTO_CONNECT_IDE`: **환경변수를 먼저 읽고**, 변수가
  unset일 때만 설정 키(`model`, `autoConnectIde`)를 사용합니다.
- 그 외에는 기능마다 다르므로 각 변수/키 문서를 확인해야 합니다.
  (예: `--model`·`/model`은 `ANTHROPIC_MODEL`을 이깁니다. 반대로
  `CLAUDE_CODE_EFFORT_LEVEL`은 `--effort`·`/effort`를 이깁니다.)
- **설정 파일의 `env` 값이 셸 변수보다 우선**하며 상속된 값을 대체합니다.
  셸 변수를 무력화하려면 `"VAR_NAME": ""`로 빈 문자열을 지정합니다.
- 셸 변수는 **시작 시 1회만** 읽으므로 변경하려면 `claude`를 재실행해야 합니다.
  설정 파일의 `env`는 파일이 바뀌면 실행 중 세션에도 재적용됩니다(OpenTelemetry 등 시작 전용 기능 제외).

> [!WARNING]
> 실행 파일 문자열에서는 `CLAUDE_*` 변수가 700개 이상 확인되지만 대부분 문서화되지 않은
> 내부 실험 플래그이며 버전 간 예고 없이 바뀝니다. spona 프리셋은 위 공식 문서 표의 항목만
> 1급 필드로 다루고, 나머지는 사용자가 직접 지정하는 자유 형식 `env` 맵으로 통과시키는 편이
> 안전합니다.
