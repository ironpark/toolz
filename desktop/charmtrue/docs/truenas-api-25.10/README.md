# TrueNAS API 25.10

> 기준 버전: TrueNAS API **v25.10.5**
> 확인일: 2026-09-02
> 공식 문서: <https://api.truenas.com/v25.10/>

CharmTrue의 TrueNAS 25.10 연동을 위한 개발 문서다. 공통 프로토콜은 `guides/`,
전체 공개 API는 기능별로 `domains/`에 정리한다. 메서드의 인자와 반환 JSON Schema는
각 항목에서 연결한 공식 상세 페이지를 정본으로 삼는다.

## 공통 가이드

1. [전송과 호출](guides/transport.md)
2. [인증과 보안](guides/authentication.md)
3. [쿼리, 작업, 이벤트](guides/query-jobs-events.md)
4. [오류와 RBAC](guides/errors-rbac.md)
5. [CharmTrue 구현 기준](guides/charmtrue.md)

## 기능별 API

| 영역 | 문서 | 주요 범위 |
| --- | --- | --- |
| 인증·계정 | [identity.md](domains/identity.md) | 인증, API 키, 사용자, 그룹, 권한 |
| 시스템 | [system.md](domains/system.md) | 시스템 설정, 부팅, 업데이트, 서비스, 일정 |
| 스토리지 | [storage.md](domains/storage.md) | 풀, 데이터셋, 스냅샷, 디스크, 파일시스템 |
| 파일 공유 | [sharing.md](domains/sharing.md) | SMB, NFS, FTP, SSH, rsync |
| 블록 스토리지 | [block-storage.md](domains/block-storage.md) | iSCSI, NVMe-oF, FC, RDMA, JBOF |
| 네트워크 | [network.md](domains/network.md) | 인터페이스, 전역 네트워크, 라우팅, DNS |
| 앱·컨테이너 | [apps.md](domains/apps.md) | 앱, 카탈로그, Docker |
| 가상화 | [virtualization.md](domains/virtualization.md) | VM, 장치, VMware 연동 |
| 백업·복제 | [data-protection.md](domains/data-protection.md) | 클라우드 백업, 동기화, 복제, 키 관리 |
| 디렉터리 서비스 | [directory-services.md](domains/directory-services.md) | 디렉터리, ID 매핑, Kerberos |
| 인증서·Web UI | [certificates-webui.md](domains/certificates-webui.md) | 인증서, ACME, Web UI |
| 관찰·지원 | [observability.md](domains/observability.md) | 알림, 보고, 감사, 메일, SNMP, UPS |
| HA·하드웨어 | [ha-hardware.md](domains/ha-hardware.md) | 페일오버, IPMI, 하드웨어 조회 |
| 원격 관리 | [remote-management.md](domains/remote-management.md) | TrueNAS Connect, TrueCommand |
| RPC 핵심 | [core.md](domains/core.md) | 구독, Job, 다운로드, 메타데이터, ping |

## 범위

- 공식 25.10.5 인덱스의 **117개 네임스페이스와 메서드 트리 항목 743개**를 포함한다.
- 이 중 741개는 호출 스키마가 있는 메서드다. `pool.scrub`와 `system.reboot` 두 항목은
  메서드 트리에 함께 노출되지만 하위 메서드를 묶는 네임스페이스 페이지이므로
  Params/Returns를 `N/A (namespace)`로 표시한다.
- 각 메서드는 공식 25.10 상세 문서로 직접 연결한다.
- 실제 인자, 반환값, Job 여부와 권한은 링크된 상세 스키마가 우선한다.
- 25.10 이후 버전의 기능은 섞지 않는다.
