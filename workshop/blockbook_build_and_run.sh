#!/usr/bin/env sh

set -e

HERE=$(dirname $(readlink -f $0))
ROOT_DIR=$(dirname $HERE)

echo "Going to $ROOT_DIR..."
cd $ROOT_DIR

echo "Building blockbook..."
go build

echo "Generating bitcoin regtest config..."
./contrib/scripts/build-blockchaincfg.sh bitcoin_regtest

echo "Running blockbook..."
./blockbook -sync -blockchaincfg=build/blockchaincfg.json -internal=:9030 -public=:9130 -logtostderr -enablesubnewtx
