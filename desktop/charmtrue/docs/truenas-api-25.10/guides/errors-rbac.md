# 오류와 RBAC

[← 문서 홈](../README.md)

## 오류

| 코드 | 의미 | 클라이언트 처리 |
| ---: | --- | --- |
| `-32000` | 동시 호출 과다 | 동시성 제한 후 지수 백오프로 재시도 |
| `-32001` | 메서드 호출 오류 | `error.data.errname`, `reason`, trace 보존 |

네트워크 오류, RPC 오류, 입력 검증 오류, Job 실패를 서로 다른 오류 유형으로 유지한다.
쓰기 작업은 멱등성을 확인하지 않고 자동 재시도하지 않는다.

## RBAC

- `READONLY_ADMIN`: 모든 읽기 역할
- `SHARING_ADMIN`: 읽기와 데이터셋·공유 관리
- `REPLICATION_ADMIN`: 복제, 스냅샷, 키체인 관련 관리
- `DISK_READ`, `SHARING_SMB_WRITE` 같은 개별 역할을 최소 권한으로 조합

운영에서는 `FULL_ADMIN`보다 전용 서비스 계정을 우선한다. 정확한 역할 매핑은
[공식 RBAC 문서](https://api.truenas.com/v25.10/rbac.html)를 확인한다.
