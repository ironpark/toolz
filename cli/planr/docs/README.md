# planr 문서

처음 사용하는 경우 상위 [README](../README.md)의 빠른 시작부터 따라가면 됩니다.

| 문서 | 다루는 내용 |
| --- | --- |
| [명령어 가이드](commands.md) | 모든 명령, JSON 출력, phase 상태 변경, 보관, 자동 완성 |
| [설정과 저장 방식](configuration.md) | `.planr.yaml`, 다중 저장 경로, 언어, 훅, 완료 기록 |
| [plan 문서 규격](document-format.md) | 초안과 등록 문서의 구조, phase 메타데이터, 의존성 규칙 |
| [개발 및 평가](development.md) | Go·Python 테스트, 재현 시나리오, Codex 평가 하네스 |

코드와 문서의 규격이 어긋났는지 확인할 때는 아래 명령을 먼저 사용합니다.

```sh
planr schema
planr doctor
```
