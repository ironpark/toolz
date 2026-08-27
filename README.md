<p align="center">
  <img src="assets/toolz.svg" width="760" alt="toolz — small tools, kept useful">
</p>

<p align="center">
  작게 시작한 도구를 실험하고, 다듬고, 쓸 만한 형태로 공개하는 모노레포
</p>

<p align="center">
  <a href="#상태">상태</a> ·
  <a href="#cli">CLI</a> ·
  <a href="#에이전트-스킬">에이전트 스킬</a> ·
  <a href="#보너스-트랙">보너스 트랙</a> ·
  <a href="#라이선스">라이선스</a>
</p>

> [!IMPORTANT]
> 이 저장소의 도구는 필요에 따라 삭제되거나 독립 저장소로 이전될 수 있으며,
> 일부는 미완성이거나 동작이 불안정할 수 있습니다.

> [!NOTE]
> 도구와 스킬 이름 앞의 아이콘은 현재 개발 단계와 사용 준비 수준을 나타냅니다.
>
> - ![](assets/status/initial.svg) **초기** 아이디어와 기본 구조를 세우는 단계로, 기능과 인터페이스가 크게 바뀔 수 있습니다.
> - ![](assets/status/development.svg) **개발** 주요 기능을 구현하고 있으며, 일부 기능은 동작하지만 변경이 잦을 수 있습니다.
> - ![](assets/status/validation.svg) **검증** 핵심 구현을 마치고 실제 환경의 동작과 호환성을 확인하고 있습니다.
> - ![](assets/status/ready.svg) **사용** 주요 기능을 사용할 수 있으며, 지속적으로 유지보수합니다.

## CLI

[`cli/`](cli/)에는 구현 언어나 런타임에 관계없이 독립적으로 설치하고 실행할 수 있는
CLI 프로젝트를 모아 관리합니다. 각 도구는 자체 설정과 README를 갖습니다.

- ![](assets/status/development.svg) **[`chatctl`](cli/chatctl/)** ChatGPT, Gemini, Claude 웹 세션의 대화 목록을 한곳에서 관리합니다.
- ![](assets/status/initial.svg) **[`mohae`](cli/mohae/)** CLI, MCP 서버, 에이전트 스킬을 재현 가능한 환경에서 실행하고 평가합니다.
- ![](assets/status/ready.svg) **[`planr`](cli/planr/)** Markdown 계획을 규격화해 저장하고 phase 단위 진행 상태를 관리합니다.
- ![](assets/status/validation.svg) **[`sanbo`](cli/sanbo/)** [`getpaseo/paseo-relay`](https://github.com/getpaseo/paseo-relay)의 드롭인 교체를 목표로 합니다.

## 에이전트 스킬

반복 작업을 에이전트가 일관되게 수행하도록 만드는 스킬입니다. 플랫폼별 소스는
[`skillz/`](skillz/) 아래에서 관리하며, 설치 스크립트가 각 플랫폼의 스킬 디렉터리에
심볼릭 링크를 연결합니다.

- ![](assets/status/ready.svg) **commit** · [Codex](skillz/codex/commit/) · [Claude](skillz/claude/commit/) — 변경 사항을 검토하고 의도한 변경만 스테이징해 정확한 Git 커밋을 만듭니다.

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

|  대상  | 기본 설치 위치     | 경로 변경               |
| :----: | ------------------ | ----------------------- |
| Codex  | `~/.codex/skills`  | `CODEX_HOME` 환경 변수  |
| Claude | `~/.claude/skills` | `CLAUDE_HOME` 환경 변수 |

기존의 일반 파일이나 디렉터리는 덮어쓰지 않습니다.

## 보너스 트랙

> [!TIP]
> 이곳에서 시작해 독립 프로젝트로 성장한 도구는 별도 저장소에서 계속 개발합니다.

- **[zapp](https://github.com/ironpark/zapp)** macOS 앱을 패키징하고 배포하는 CLI
- **[macc](https://github.com/ironpark/macc)** macOS 시스템 설정을 제어하는 CLI

## 라이선스

별도 라이선스가 명시되지 않은 모든 도구는 [MIT 라이선스](LICENSE)를 따릅니다.
