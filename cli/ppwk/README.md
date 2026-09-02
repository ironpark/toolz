# ppwk

`ppwk`는 Go로 작성하는 CLI 프로젝트입니다.

> [!NOTE]
> 현재는 Go 모듈과 CLI 진입점만 준비된 초기 개발 단계입니다.

## 개발

Go 1.26.3 이상이 필요합니다.

```sh
go test ./...
go run . --version
```

실행 파일은 다음과 같이 빌드할 수 있습니다.

```sh
go build -o bin/ppwk .
```

## 라이선스

별도 명시가 없는 한 저장소 루트의 [MIT 라이선스](../../LICENSE)를 따릅니다.
