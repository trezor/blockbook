#!/bin/bash
set -e

cp -r /src/build/deb/debian .
cp -r /src/configs .
mkdir server && cp -r /src/server/testcert.* /src/server/static server

dpkg-buildpackage -us -uc
mv ../*.deb /out
chown $PACKAGER /out/*.deb
