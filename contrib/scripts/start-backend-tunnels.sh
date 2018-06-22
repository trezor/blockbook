#!/bin/bash

if [ $# -ne 1 ]; then
    echo "Usage: $(basename $0) host" 1>&2
    exit 1
fi

host=$1

# change dir to root of git repository
cd $(cd $(dirname $(readlink -f $0)) && git rev-parse --show-toplevel)

# get all testnet ports from configs/
testnet_ports=$(gawk 'match($0, /"rpcURL":\s+"(http|ws):\/\/[^:]+:([0-9]+)"/, a) {print a[2]}' configs/*_testnet*.json)

for port in $testnet_ports
do
    ssh -nNT -L $port:localhost:$port $host &
    pid=$!
    echo "Started tunnel to ${host}:${port} (pid: ${pid})"
done

at_exit() {
    pkill -P $$
}

trap at_exit EXIT

wait -n
code=$?

if [ $code != 0 ]; then
    exit $code
fi
