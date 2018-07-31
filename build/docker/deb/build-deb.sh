#!/bin/bash
set -e

if [ $# -lt 2 ]; then
    echo "Missing arugments" 1>&2
    echo "Usage: $(basename $0) <backend|blockbook|all> <coin> [build opts]" 1>&2
    exit 1
fi

package=$1
coin=$2
shift 2

cp -r /src/build/templates .
cp -r /src/configs .
go run templates/generate.go $coin

# backend
if [ $package = "backend" ] || [ $package = "all" ]; then
    (cd pkg-defs/backend && dpkg-buildpackage -us -uc $@)
fi

# blockbook
if [ $package = "blockbook" ] || [ $package = "all" ]; then
    export VERSION=$(cd pkg-defs/blockbook && dpkg-parsechangelog | sed -rne 's/^Version: ([0-9.]+)([-+~].+)?$/\1/p')

    cp Makefile ldb sst_dump pkg-defs/blockbook
    cp -r /src/static pkg-defs/blockbook
    mkdir pkg-defs/blockbook/cert && cp /src/server/testcert.* pkg-defs/blockbook/cert
    (cd pkg-defs/blockbook && dpkg-buildpackage -us -uc $@)
fi

# copy packages
mv pkg-defs/*.deb /out
chown $PACKAGER /out/*.deb
