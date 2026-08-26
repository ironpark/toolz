# toolz

개인적으로 만든 도구를 모아 공개하고 관리하는 모노레포입니다.
이 저장소의 도구는 필요에 따라 삭제되거나, 독립 저장소로 이전될 수 있습니다.
일부 도구는 미완성이거나 동작이 불안정할 수 있습니다.

## 도구 목록

| 도구 | 설명 |
| --- | --- |
| [`planr`](planr/) | 규격화된 Markdown 계획을 등록·조회하는 Go CLI입니다. |

각 도구의 사용 방법과 상세 설명은 해당 디렉터리에서 안내합니다.

## Codex 스킬

| 스킬 | 설명 |
| --- | --- |
| [`commit`](skills/commit/) | 변경 사항을 검토·스테이징하고 정확한 Git 커밋을 만듭니다. |

스킬은 [`skills/`](skills/)에서 관리합니다. Codex에 설치하려면 다음을 실행합니다.

```sh
./scripts/install-skills.sh
```

## 별도 저장소 도구

| 도구 | 설명 |
| --- | --- |
| [zapp](https://github.com/ironpark/zapp) | macOS 앱 배포를 위한 CLI 도구입니다. |
| [macc](https://github.com/ironpark/macc) | macOS 시스템 설정을 제어하는 CLI 도구입니다. |
