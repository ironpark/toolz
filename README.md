<p align="center">
  <img src="assets/toolz.svg" width="760" alt="toolz — small tools, kept useful">
</p>

<p align="center">
  작게 시작한 도구를 실험하고, 다듬고, 쓸 만한 형태로 공개하는 모노레포
</p>

<p align="center">
  <a href="#cli">CLI</a> ·
  <a href="#에이전트-스킬">에이전트 스킬</a> ·
  <a href="#보너스-트랙">보너스 트랙</a> ·
  <a href="#라이선스">라이선스</a>
</p>

> [!IMPORTANT]
> 이 저장소의 도구는 필요에 따라 삭제되거나 독립 저장소로 이전될 수 있으며,
> 일부는 미완성이거나 동작이 불안정할 수 있습니다.

각 프로젝트는 독립적인 Go 모듈입니다. 관심 있는 도구의 디렉터리로 이동하면 다른
프로젝트와 분리해 빌드하고 테스트할 수 있습니다.

## CLI

| 도구 | 하는 일 |
| :--- | --- |
| ![](assets/status/development.svg) **[`chatctl`](cli/chatctl/)** | ChatGPT, Gemini, Claude 웹 세션의 대화 목록을 한곳에서 관리합니다. |
| ![](assets/status/initial.svg) **[`mohae`](cli/mohae/)** | CLI, MCP 서버, 에이전트 스킬을 재현 가능한 환경에서 실행하고 평가합니다. |
| ![](assets/status/ready.svg) **[`planr`](cli/planr/)** | Markdown 계획을 규격화해 저장하고 phase 단위 진행 상태를 관리합니다. |
| ![](assets/status/validation.svg) **[`sanbo`](cli/sanbo/)** | [`getpaseo/paseo-relay`](https://github.com/getpaseo/paseo-relay)의 드롭인 교체를 목표로 합니다. |

- ![](assets/status/initial.svg) **초기** — 기본 구조와 방향을 잡는 단계
- ![](assets/status/development.svg) **개발** — 주요 기능을 구현하는 단계
- ![](assets/status/validation.svg) **검증** — 기능과 호환성을 확인하는 단계
- ![](assets/status/ready.svg) **사용** — 주요 기능을 사용할 수 있는 단계

## 에이전트 스킬

반복 작업을 에이전트가 일관되게 수행하도록 만드는 스킬입니다. 플랫폼별 소스는
[`skillz/`](skillz/) 아래에서 관리하며, 설치 스크립트가 각 플랫폼의 스킬 디렉터리에
심볼릭 링크를 연결합니다.

| 플랫폼 | 스킬 | 설명 |
| :---: | --- | --- |
| Codex | [`commit`](skillz/codex/commit/) | 변경 사항을 검토·스테이징하고 정확한 Git 커밋을 만듭니다. |
| Claude | [`commit`](skillz/claude/commit/) | 변경 사항을 검토하고 의도한 변경만 정확한 Git 커밋으로 만듭니다. |

### 설치

Codex와 Claude 스킬을 모두 설치합니다.

```sh
./scripts/install-skills.sh
```

특정 플랫폼만 선택할 수도 있습니다.

```sh
./scripts/install-skills.sh codex
./scripts/install-skills.sh claude
```

| 대상 | 기본 설치 위치 | 경로 변경 |
| :---: | --- | --- |
| Codex | `~/.codex/skills` | `CODEX_HOME` 환경 변수 |
| Claude | `~/.claude/skills` | `CLAUDE_HOME` 환경 변수 |

기존의 일반 파일이나 디렉터리는 덮어쓰지 않습니다.

## 보너스 트랙

> [!TIP]
> 이곳에서 시작해 독립 프로젝트로 성장한 도구는 별도 저장소에서 계속 개발합니다.

| 도구 | 설명 |
| --- | --- |
| **[zapp](https://github.com/ironpark/zapp)** | macOS 앱을 패키징하고 배포하는 CLI |
| **[macc](https://github.com/ironpark/macc)** | macOS 시스템 설정을 제어하는 CLI |

## 라이선스

별도 라이선스가 명시되지 않은 모든 도구는 [MIT 라이선스](LICENSE)를 따릅니다.
