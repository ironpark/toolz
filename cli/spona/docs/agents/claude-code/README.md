# Claude Code (`claude`)

- 조사 버전: `2.1.257 (Claude Code)`
- 1차 출처(공식 문서)
  - [CLI 레퍼런스](https://code.claude.com/docs/en/cli-reference)
  - [설정](https://code.claude.com/docs/en/settings) · [설정 키 레퍼런스](https://code.claude.com/docs/en/settings-reference)
  - [환경변수 레퍼런스](https://code.claude.com/docs/en/env-vars)
  - [훅](https://code.claude.com/docs/en/hooks) · [서브에이전트](https://code.claude.com/docs/en/sub-agents)
  - [프로그래매틱 실행](https://code.claude.com/docs/en/headless) · [터미널 설정](https://code.claude.com/docs/en/terminal-config)
- 2차 출처: 로컬 설치본의 `claude --help` 출력 (공식 문서에 없는 항목 교차 확인용)

## 문서

| 문서 | 내용 |
| --- | --- |
| [cli.md](cli.md) | 모델·지침·도구/권한·설정 소스·격리 모드·입출력·세션 수명주기 플래그, 서브커맨드 |
| [settings.md](settings.md) | `settings.json` 파일 스코프와 우선순위, 설정 키 그룹별 목록 |
| [env.md](env.md) | 인증·프로바이더·모델·네트워크·경로 환경변수, 설정 키와의 우선순위 |
| [agents.md](agents.md) | 서브에이전트 정의 파일 포맷, `--agents` JSON, 모델 해석 순서 |
| [hooks.md](hooks.md) | 훅 정의 위치·구조·이벤트·입출력·종료 코드 |

교차 문서: [../interactive.md](../interactive.md) (대화형 실행) ·
[../spawn.md](../spawn.md) (비대화형 실행)

## 기본 사용법

```
claude [options] [command] [prompt]
```

인자 없이 실행하면 **대화형 세션**, `-p/--print`를 주면 비대화형으로 응답만 출력하고
종료합니다. spona의 기본 경로는 전자입니다.

```sh
# 대화형 (spona 기본) — TTY를 그대로 물려줌
CLAUDE_CONFIG_DIR=<preset-home> claude \
  --model claude-sonnet-5 \
  --permission-mode acceptEdits \
  --name "$PRESET_NAME" \
  --settings "$PRESET_JSON"
```

```sh
# 비대화형 (결과 회수용 보조 경로)
claude -p "프롬프트" \
  --model claude-sonnet-5 \
  --output-format stream-json --verbose \
  --permission-mode dontAsk \
  --max-turns 20 \
  --settings '{"...": "..."}'
```

## spona가 특히 주의할 점

- **stdout이 TTY가 아니면** 워크스페이스 신뢰 다이얼로그를 건너뛰고, 검증에 실패한 설정
  파일도 오류 없이 조용히 무시합니다. → [../interactive.md](../interactive.md)
- `--bare`는 스크립트·CI의 권장 모드이지만 **대화형 기본값으로는 부적절**합니다
  (훅·스킬·플러그인·MCP·`CLAUDE.md`가 모두 빠지고 OAuth·키체인을 읽지 않음).
- `--mcp-config`의 무효 항목과 로드 실패한 플러그인은 **조용히 건너뛰고 정상 종료**합니다.
  `system/init` 이벤트의 `mcp_server_errors` / `plugin_errors`로 확인해야 합니다.
- 설정 홈은 `CLAUDE_CONFIG_DIR`로 재지정합니다(바이너리 문자열에 보이는
  `ANTHROPIC_CONFIG_DIR`이 아님).
- `claude import codex` 가 이미 존재합니다 — spona의 설정 변환 기능과 역할이 겹칩니다.
