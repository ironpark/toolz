#!/bin/sh
# 프론트엔드를 빌드해 Go 패키지 안에 넣는다.
#
# 산출물(internal/web/dist)은 저장소에 커밋한다. go:embed 는 빌드 시점에
# 파일이 있어야 하므로, 커밋하지 않으면 node 없이는 go build 조차 되지
# 않는다. CLI 하나만 받으면 되는 성질을 지키기 위한 값이다.
set -eu
cd "$(dirname "$0")/.."
# 이전 산출물을 지운다. 파일 이름에 해시가 붙으므로 지우지 않으면 옛 청크가
# 계속 쌓이고, 그것이 그대로 바이너리에 박힌다.
rm -rf internal/web/dist

cd front
pnpm install --frozen-lockfile
pnpm check
pnpm build
