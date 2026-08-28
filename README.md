<p align="center">
  <img src="assets/toolz.svg" width="760" alt="toolz — small tools, kept useful">
</p>

<p align="center">
  작게 시작한 도구를 실험하고, 다듬고, 쓸 만한 형태로 공개하는 모노레포
</p>

<p align="center">
  <a href="#저장소-구조">구조</a> ·
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
> - ![](assets/status/initial.svg) **초기** 아이디어를 구체화하고 구조를 잡는 단계입니다.
> - ![](assets/status/development.svg) **개발** 핵심 기능을 구현중입니다. 일부 기능은 동작할 수도 있습니다.
> - ![](assets/status/validation.svg) **검증** 기능과 품질을 확인하는 단계입니다.
> - ![](assets/status/ready.svg) **사용** 사용 가능한 상태입니다.
> - ![](assets/status/stable.svg) **안정** 기능과 인터페이스가 안정된 상태입니다.

## 저장소 구조

서로 다른 작은 도구를 한곳에서 관리하되, 각 프로젝트가 독립적으로 발전할 수 있도록
용도별 디렉터리로 나눕니다. `cli/`의 도구는 구현 언어나 런타임과 관계없이 개별적으로
설치·실행할 수 있으며, 자세한 사용법은 각 프로젝트의 README에서 안내합니다.

```text
toolz/
├── cli/       독립적으로 실행하는 CLI 프로젝트
├── skillz/    에이전트 플랫폼별 스킬 소스
├── scripts/   저장소 관리와 설치 스크립트
└── assets/    README와 프로젝트 공용 이미지
```

## CLI

- ![](assets/status/development.svg) **[`chatctl`](cli/chatctl/)** ChatGPT, Gemini, Claude 웹 세션에 저장된 대화 목록을 조회하고 관리합니다.
- ![](assets/status/initial.svg) **[`mohae`](cli/mohae/)** CLI, MCP 서버, 에이전트 스킬을 재현 가능한 환경에서 실행하고 비교·평가합니다.
- ![](assets/status/ready.svg) **[`planr`](cli/planr/)** Markdown 계획을 규격화해 저장하고 phase 단위로 진행 상태를 관리합니다.
- ![](assets/status/validation.svg) **[`sanbo`](cli/sanbo/)** [`getpaseo/paseo-relay`](https://github.com/getpaseo/paseo-relay)의 공개 프로토콜·운영 설정과 호환되는 드롭인 교체를 목표로 합니다.

## 에이전트 스킬

- ![](assets/status/ready.svg) **commit** ([Codex](skillz/codex/commit/) · [Claude](skillz/claude/commit/)) — 변경 사항을 검토하고 의도한 변경만 스테이징해 정확한 Git 커밋을 만듭니다.

### 스킬 설치와 제거

[스킬 스크립트](scripts/skills.sh)는 저장소의 스킬을 각 플랫폼 디렉터리에
심볼릭 링크로 연결합니다. 대상을 생략하면 Codex와 Claude 스킬을 모두 처리합니다.

```sh
./scripts/skills.sh install
```

특정 플랫폼만 선택해 설치할 수도 있습니다.

```sh
./scripts/skills.sh install codex
./scripts/skills.sh install claude
```

설치 상태를 확인하거나 다시 제거할 수 있습니다.

```sh
./scripts/skills.sh status
./scripts/skills.sh uninstall
```

|  대상  | 기본 설치 위치     | 경로 변경               |
| :----: | ------------------ | ----------------------- |
| Codex  | `~/.codex/skills`  | `CODEX_HOME` 환경 변수  |
| Claude | `~/.claude/skills` | `CLAUDE_HOME` 환경 변수 |

기존의 일반 파일이나 디렉터리는 덮어쓰지 않습니다. 저장소를 다른 경로로 옮겼다면
새 위치에서 `install`을 다시 실행해 심볼릭 링크를 갱신합니다. `uninstall`은 이
저장소를 가리키는 링크만 제거하며, 다른 대상을 가리키는 링크나 일반 파일은 건드리지
않습니다.

## 보너스 트랙

> [!TIP]
> 이곳에서 시작해 독립 프로젝트로 성장한 도구는 별도 저장소에서 계속 개발합니다.

- ![](assets/status/stable.svg) **[zapp](https://github.com/ironpark/zapp)** macOS 앱을 패키징하고 배포하는 CLI
- ![](assets/status/validation.svg) **[macc](https://github.com/ironpark/macc)** macOS 시스템 설정을 제어하는 CLI

## 라이선스

각 도구는 해당 디렉터리에 명시된 라이선스를 우선 적용합니다. 별도 라이선스가 없는
도구는 저장소 루트의 [MIT 라이선스](LICENSE)를 따릅니다.
