#!/bin/bash
set -e

cp -r /src/build/deb/debian .
cp -r /src/configs .
cp -r /src/static static
mkdir cert && cp /src/server/testcert.* cert

dpkg-buildpackage -us -uc
mv ../*.deb /out
chown $PACKAGER /out/*.deb
