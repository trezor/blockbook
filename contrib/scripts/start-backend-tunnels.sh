#!/usr/bin/env bash

if [ $# -ne 1 ]; then
    echo "Usage: $(basename $0) host" 1>&2
    exit 1
fi

host=$1

get_port() {
    data=$1
    key=$2
    echo "${data}" | gawk "match(\$0, /\"${key}\":\s+([0-9]+)/, a) {print a[1]}" -
}

# change dir to root of git repository
cd $(cd $(dirname $(readlink -f $0)) && git rev-parse --show-toplevel)

# get all testnet ports from configs/
ports=$(gawk 'match($0, /"backend_rpc":\s+([0-9]+)/, a) {print a[1]}' configs/coins/*.json)

for port in $ports
do
    ssh -nNT -L $port:localhost:$port $host &
    pid=$!
    echo "Started tunnel to ${host}:${port} (pid: ${pid})"
done

at_exit() {
    pkill -P $$
}

trap at_exit EXIT

sleep inf
# wait -n
# code=$?
#
# if [ $code != 0 ]; then
#     exit $code
# fi
