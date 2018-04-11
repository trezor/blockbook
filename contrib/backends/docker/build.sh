#!/bin/bash
set -e

if [ $# -ne 1 ]
then
    echo "Usage: $(basename $0) target" > /dev/stderr
    exit 1
fi

cd $1

mk-build-deps -ir -t "apt-get -qq --no-install-recommends"
dpkg-buildpackage -us -uc
mv ../*.deb .
chown $PACKAGER *.deb
