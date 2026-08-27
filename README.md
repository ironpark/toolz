# toolz

> [!IMPORTANT]
> 이 저장소의 도구는 필요에 따라 삭제되거나 독립 저장소로 이전될 수 있으며,
> 일부는 미완성이거나 동작이 불안정할 수 있습니다.

개인적으로 만든 도구를 모아 공개하고 관리하는 모노레포입니다.

## CLI

| 도구 | 설명 |
| --- | --- |
| [`chatctl`](cli/chatctl/) | ChatGPT, Gemini, Claude 웹 세션의 대화 목록을 관리하는 Go CLI입니다. |
| [`mohae`](cli/mohae/) | CLI, MCP 서버, 에이전트 스킬을 재현 가능한 환경에서 평가하는 Go CLI입니다. |
| [`planr`](cli/planr/) | 규격화된 Markdown 계획을 등록·조회하는 Go CLI입니다. |
| [`sanbo`](cli/sanbo/) | WebSocket 연결을 중계하고 라우팅하는 Go CLI입니다. |

각 도구의 사용 방법과 상세 설명은 해당 디렉터리에서 안내합니다.

## 에이전트 스킬

플랫폼별 스킬은 [`skillz/`](skillz/) 아래에서 관리합니다.

| 플랫폼 | 스킬 | 설명 |
| --- | --- | --- |
| Codex | [`commit`](skillz/codex/commit/) | 변경 사항을 검토·스테이징하고 정확한 Git 커밋을 만듭니다. |
| Claude | [`commit`](skillz/claude/commit/) | 변경 사항을 검토하고 의도한 변경만 정확한 Git 커밋으로 만듭니다. |

### 설치

설치 스크립트는 기본적으로 Codex와 Claude 스킬을 모두 심볼릭 링크로 설치합니다.

```sh
./scripts/install-skills.sh
```

플랫폼 하나만 설치할 수도 있습니다.

```sh
./scripts/install-skills.sh codex
./scripts/install-skills.sh claude
```

기본 설치 위치는 각각 `~/.codex/skills`와 `~/.claude/skills`입니다. 다른 홈을 사용하려면
`CODEX_HOME` 또는 `CLAUDE_HOME` 환경 변수를 지정합니다. 기존의 일반 파일이나 디렉터리는
덮어쓰지 않습니다.

## 보너스 트랙

> [!TIP]
> 이 저장소에서 시작해 별도 저장소로 분리된 도구들입니다.

| 도구 | 설명 |
| --- | --- |
| [zapp](https://github.com/ironpark/zapp) | macOS 앱 배포를 위한 CLI 도구입니다. |
| [macc](https://github.com/ironpark/macc) | macOS 시스템 설정을 제어하는 CLI 도구입니다. |

## 라이선스

각 도구는 자신의 디렉터리에 있는 라이선스 파일을 따릅니다. 해당 디렉터리에 별도의
라이선스 파일이 없으면 저장소 루트의 [MIT 라이선스](LICENSE)가 적용됩니다.
현재 별도 라이선스를 두는 도구는 없으므로 모든 도구가 MIT를 따릅니다.
