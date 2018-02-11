#!/usr/bin/env bash
set -e

cd `dirname $0`

docker build -t blockbook-build .
docker run -t -v $(pwd):/out blockbook-build /bin/cp /go/src/blockbook/blockbook /out/

strip blockbook
