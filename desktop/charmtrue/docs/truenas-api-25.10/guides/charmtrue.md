# CharmTrue 구현 기준

[← 문서 홈](../README.md)

`internal/truenas` 전송 계층은 주소 정규화, TLS 경고, `auth.login_ex` 인증, 단일 연결의
동시 호출과 응답 ID 매칭, 구조화된 오류·Job·알림 전달, 컨텍스트 취소, 재연결·재인증·
구독 복원을 담당한다. 도메인 모델과 고수준 서비스는 그 위에 분리한다.

파괴적인 `delete`, `wipe`, `reset`, `rollback`, `reboot` 계열은 타입 또는 메타데이터로
표시해 UI 확인을 강제한다.

| 이름 | 일반적인 의미 |
| --- | --- |
| `query` | 필터 가능한 목록 조회 |
| `get_instance` | ID로 단일 객체 조회 |
| `config` | 단일 설정 객체 조회 |
| `create` / `update` / `delete` | 생성 / 변경 / 삭제 |
| `*_choices` | 서버가 허용하는 선택지 조회 |

이는 관례다. 실제 인자, 반환값, Job 여부와 권한은 공식 상세 페이지를 확인한다.
