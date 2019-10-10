#!/usr/bin/env bash

if [ $# -lt 2 ]
then
    echo "Usage: $(basename $(readlink -f $0)) hostname coin [...]" 1>&2
    exit 1
fi


HOST=$1
shift
COINS=$@

REPO=$(cd $(dirname $(readlink -f $0)) && git rev-parse --show-toplevel)
UPDATE_VENDOR=${UPDATE_VENDOR:0}

cd ${REPO}

VERSION=$(cd build/deb && dpkg-parsechangelog | sed -rne 's/^Version: ([0-9.]+)([-+~].+)?$/\1/p')

make deb UPDATE_VENDOR=${UPDATE_VENDOR} || exit $?

echo -e "\nDeploying: $@\n"

status=0

for coin in $COINS
do
    scp build/blockbook-${coin}_${VERSION}_amd64.deb ${HOST}: \
        && ssh ${HOST} "sudo DEBIAN_FRONTEND=noninteractive apt-get install -y --reinstall ./blockbook-${coin}_${VERSION}_amd64.deb && sudo systemctl restart blockbook-${coin}.service" \
        || status=$?

    if [ ${status} == 0 ]
    then
        echo -e "\nOK - ${coin} deployed"
    else
        echo -e "\nFAIL - ${coin} status: ${status}"
    fi

    echo
done

make clean

echo -e "\nDONE"

exit ${status}
