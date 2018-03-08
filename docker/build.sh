#!/usr/bin/env bash
set -e

cd `dirname $0`

# prepare build image
docker build -t blockbook-build .

if [ "$1" == "local" ]; then
    SRC_BIND="-v $(pwd)/..:/go/src/blockbook"
fi

# build binary
docker run -t --rm -v $(pwd):/out $SRC_BIND blockbook-build

strip blockbook
