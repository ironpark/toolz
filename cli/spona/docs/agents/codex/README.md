# OpenAI Codex CLI (`codex`)

- 조사 버전: `codex-cli 0.153.0`
- 1차 출처(공식 문서)
  - [기본 설정](https://learn.chatgpt.com/docs/config-file/config-basic) · [고급 설정](https://learn.chatgpt.com/docs/config-file/config-advanced) · [설정 레퍼런스](https://learn.chatgpt.com/docs/config-file/config-reference)
  - [Rules (execpolicy)](https://learn.chatgpt.com/docs/agent-configuration/rules)
  - [비대화형 모드](https://learn.chatgpt.com/docs/non-interactive-mode)
  - [저장소 문서](https://github.com/openai/codex/tree/main/docs)
- 2차 출처: 로컬 설치본의 `codex --help`, `codex exec --help` 등

## 문서

| 문서 | 내용 |
| --- | --- |
| [cli.md](cli.md) | 공통 옵션, `codex exec` 전용 옵션, 서브커맨드 전체 |
| [config.md](config.md) | 설정 레이어·우선순위·신뢰 모델, `$CODEX_HOME` 구조, `config.toml` 주요 키 |
| [env.md](env.md) | 문서화된 환경변수와 바이너리에서만 확인되는 변수 구분 |
| [rules.md](rules.md) | execpolicy `.rules` — Starlark `prefix_rule`, 결정 우선순위, 검증 |
| [hooks.md](hooks.md) | 훅 정의 위치, 이벤트 키, 신뢰 등록, 관리형 훅 강제 |

교차 문서: [../interactive.md](../interactive.md) (대화형 실행) ·
[../spawn.md](../spawn.md) (비대화형 실행)

## 기본 사용법

```
codex [OPTIONS] [PROMPT]
codex [OPTIONS] <COMMAND> [ARGS]
```

**서브커맨드를 생략하면 옵션이 그대로 대화형 TUI로 전달됩니다.** 즉 spona는 `codex exec`를
쓰지 않는 것만으로 대화형이 됩니다. 비대화형 실행은 `codex exec`(별칭 `codex e`)입니다.

```sh
# 대화형 (spona 기본)
CODEX_HOME=<preset-home> codex \
  --model gpt-5.5 \
  --sandbox workspace-write \
  --ask-for-approval on-request \
  --cd /path/to/repo \
  -c model_reasoning_effort=high
```

```sh
# 비대화형 (결과 회수용 보조 경로)
codex exec "프롬프트" \
  --model gpt-5.5 \
  --sandbox workspace-write \
  --ask-for-approval never \
  --cd /path/to/repo \
  --ephemeral --ignore-user-config --ignore-rules \
  --json --output-last-message /tmp/last.txt
```

## spona가 특히 주의할 점

- **프로젝트가 untrusted면 `.codex/` 레이어 전체를 건너뜁니다.** 프로젝트 디렉터리에
  설정 파일을 써 두는 주입 방식은 조용히 무시될 수 있습니다. `CODEX_HOME` 격리 또는
  `-c` 오버라이드가 더 결정적입니다.
- `shell_environment_policy`가 기본적으로 이름에 `KEY`/`SECRET`/`TOKEN`이 든 변수를
  걸러냅니다. `os/exec`의 `Env`에 넣는 것만으로는 에이전트 셸까지 도달하지 않습니다.
- `agents.<name>.config_file` 은 "이름 붙은 역할 = 설정 레이어 파일" 구조로 spona의 프리셋
  개념과 사실상 동일합니다. 이 형태로 내보내면 Codex 자체 멀티에이전트 기능과 호환됩니다.
- 설정 대부분이 `config.toml` + `-c` 오버라이드로 표현되도록 설계돼 있으므로, 환경변수는
  `CODEX_HOME`과 자격증명 위주로만 다루는 편이 공식 모델과 일치합니다.
