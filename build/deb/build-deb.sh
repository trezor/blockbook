#!/bin/bash
set -e

cp -r /src/build/deb/debian .
cp -r /src/configs .
cp -r /src/static static
mkdir cert && cp /src/server/testcert.* cert

export VERSION=$(dpkg-parsechangelog | sed -rne 's/^Version: ([0-9.]+)([-+~].+)?$/\1/p')

dpkg-buildpackage -us -uc
mv ../*.deb /out
chown $PACKAGER /out/*.deb
