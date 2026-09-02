# CharmTrue

CharmTrue는 TrueNAS 시스템을 관리하기 위한 크로스 플랫폼 데스크톱
애플리케이션입니다. Go 백엔드와 TypeScript 프런트엔드를 Wails v3로
구성합니다.

> [!NOTE]
> 현재는 프로젝트 기반과 관리 화면만 준비된 초기 개발 단계입니다. TrueNAS
> 연결, 인증 정보 저장 및 API 기능은 이후 구현할 예정입니다.

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

프로덕션 빌드는 `bin/`에 생성됩니다.

```sh
wails3 build
```

## 라이선스

별도 명시가 없는 한 저장소 루트의 [MIT 라이선스](../../LICENSE)를 따릅니다.
