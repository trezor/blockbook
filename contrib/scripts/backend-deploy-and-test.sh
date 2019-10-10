#!/usr/bin/env bash

if [ $# -ne 1 ] && [ $# -ne 4 ]
then
    echo -e "Usage:\n\n$(basename $(readlink -f $0)) coin service_name coin_test backend_log_file\n\nor\n\n$(basename $(readlink -f $0)) coin\nin which case service_name, coin_test and backend_log_file are derived from coin or default" 1>&2
    exit 1
fi

COIN=$1
SERVICE=$2
COIN_TEST=$3
LOGFILE=$4

[ -z "${BACKEND_TIMEOUT}" ] && BACKEND_TIMEOUT=15s
[ -z "${SERVICE}" ] && SERVICE="${COIN}"
[ -z "${COIN_TEST}" ] && COIN_TEST="${COIN}=main"
[ -z "${LOGFILE}" ] && LOGFILE=debug.log

echo "Running: $(basename $(readlink -f $0)) ${COIN} ${SERVICE} ${COIN_TEST} ${LOGFILE}"

rm build/*.deb
make "deb-backend-${COIN}"

PACKAGE=$(ls ./build/backend-${SERVICE}*.deb)
[ -z "${PACKAGE}" ] && echo "Package not found" && exit 1

sudo /usr/bin/dpkg -i "${PACKAGE}" || exit 1
sudo /bin/systemctl restart "backend-${SERVICE}" || exit 1

echo "Waiting for backend startup for ${BACKEND_TIMEOUT}"
sudo -u bitcoin /usr/bin/timeout ${BACKEND_TIMEOUT} /usr/bin/tail -f "/opt/coins/data/${COIN}/backend/${LOGFILE}"

make test-integration ARGS="-v -run=TestIntegration/${COIN_TEST}"
