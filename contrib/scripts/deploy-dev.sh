#!/usr/bin/env bash
set -euo pipefail

prog="$(basename "$(readlink -f "$0")")"

if [ $# -lt 2 ]
then
    echo "Usage: ${prog} hostname coin [...]" 1>&2
    exit 1
fi


HOST="$1"
shift
COINS=("$@")

REPO="$(cd "$(dirname "$(readlink -f "$0")")" && git rev-parse --show-toplevel)"
UPDATE_VENDOR="${UPDATE_VENDOR:-0}"

cd "${REPO}"

VERSION="$(cd build/deb && dpkg-parsechangelog | sed -rne 's/^Version: ([0-9.]+)([-+~].+)?$/\1/p')"

make deb "UPDATE_VENDOR=${UPDATE_VENDOR}"

echo -e "\nDeploying: ${COINS[*]}\n"

status=0

for coin in "${COINS[@]}"
do
    coin_status=0
    pkg="build/blockbook-${coin}_${VERSION}_amd64.deb"
    scp "${pkg}" "${HOST}:" \
        && ssh "${HOST}" "pkg=\$PWD/blockbook-${coin}_${VERSION}_amd64.deb && sudo DEBIAN_FRONTEND=noninteractive apt install -y --reinstall \"\$pkg\" && sudo systemctl restart blockbook-${coin}.service" \
        || coin_status=$?

    if [ "${coin_status}" = 0 ]
    then
        echo -e "\nOK - ${coin} deployed"
    else
        status="${coin_status}"
        echo -e "\nFAIL - ${coin} status: ${coin_status}"
    fi

    echo
done

make clean

echo -e "\nDONE"

exit "${status}"
