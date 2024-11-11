#!/usr/bin/env sh

set -e

HERE=$(dirname $(readlink -f $0))

REGTEST_CONFIG=$HERE/bitcoin.conf

TARGET_DIR=~/.bitcoin

TARGET_FILE=$TARGET_DIR/bitcoin.conf

echo "Creating target directory $TARGET_DIR if it doesn't exist..."
mkdir -p $TARGET_DIR

echo "Copying regtest config to $TARGET_FILE..."
cp $REGTEST_CONFIG TARGET_FILE
