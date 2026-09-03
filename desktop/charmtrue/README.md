# CharmTrue

CharmTrue는 TrueNAS 시스템을 관리하기 위한 크로스 플랫폼 데스크톱
애플리케이션입니다. Go 백엔드와 SvelteKit 2·Svelte 5 정적 SPA 프런트엔드를
Wails v3로 구성합니다.

> [!NOTE]
> 현재는 관리 화면과 TrueNAS API 전송 패키지, API 키 또는 ID/비밀번호 연결 및
> 시스템 정보 조회를 구현한 초기 개발 단계입니다. 인증 정보 저장과 도메인별
> 관리 기능은 이후 구현할 예정입니다.

TrueNAS 연동 규약과 구현 기준은 [TrueNAS API 25.10 정리](docs/truenas-api-25.10.md)를
참고합니다.

프런트엔드는 `$lib/design-system`의 토큰과 공용 컴포넌트를 사용합니다. 컴포넌트
계층과 사용 규칙은 [디자인 시스템 안내](frontend/src/lib/design-system/README.md)를
참고합니다.

## 요구 사항

- Go 1.25 이상
- Node.js와 npm
- Wails CLI v3

```sh
go install github.com/wailsapp/wails/v3/cmd/wails3@latest
```

## 개발

```sh
wails3 dev
```

연결 창의 **사설 인증서 허용**을 선택하면 자체 서명 인증서도 사용할 수 있습니다.
이 옵션은 인증서 신뢰 체인과 호스트 이름 검증을 생략하므로 신뢰하는 내부
TrueNAS 시스템에서만 사용해야 합니다.

프로덕션 빌드는 `bin/`에 생성됩니다.

```sh
wails3 build
```

## 라이선스

별도 명시가 없는 한 저장소 루트의 [MIT 라이선스](../../LICENSE)를 따릅니다.
