#!/bin/bash

if [ $# -lt 1 ]
then
    echo "Usage: $(basename $(readlink -f $0)) coin [coin_test] [backend log file]" 1>&2
    exit 1
fi

COIN=$1
COIN_TEST=$2
LOGFILE=$3

[ -z "${BACKEND_TIMEOUT}" ] && BACKEND_TIMEOUT=15s
[ -z "${COIN_TEST}" ] && COIN_TEST="${COIN}=main"
[ -z "${LOGFILE}" ] && LOGFILE=debug.log

rm build/*.deb
make "deb-backend-${COIN}"

PACKAGE=$(ls "./build/backend-${COIN}*.deb")
[ -z "${PACKAGE}" ] && echo "Package not found" && exit 1

sudo /usr/bin/dpkg -i "${PACKAGE}" || exit 1
sudo /bin/systemctl restart "backend-${COIN}" || exit 1
timeout ${BACKEND_TIMEOUT} tail -f "/opt/coins/data/${COIN}/backend/${LOGFILE}"
make test-integration ARGS="-v -run=TestIntegration/${COIN_TEST}"

