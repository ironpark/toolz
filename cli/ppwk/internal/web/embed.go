// Package web 는 ppwk 보드를 브라우저에서 보고 조작하게 한다.
//
// 정적 자산은 바이너리에 박혀 있다. 별도 파일을 배포하면 CLI 하나만 있으면
// 된다는 성질이 사라지고, 버전이 어긋난 자산이 조용히 섞일 수 있다.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var distFS embed.FS

// Assets 는 빌드된 프론트엔드다.
//
// dist 는 `pnpm build` 가 만든다. 없으면 빌드가 깨지므로 자리표시자
// index.html 을 함께 둔다 — node 없이도 go build 가 되어야 한다.
func Assets() fs.FS {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// dist 는 embed 지시자가 보장한다. 여기 오면 프로그램이 깨진 것이다.
		panic("web: dist 를 열 수 없습니다: " + err.Error())
	}
	return sub
}
