#!/usr/bin/env sh

set -e

TARGET_DIR=~/.bitcoin/regtest

echo "Removing block and chainstate data in $TARGET_DIR..."
rm -rf $TARGET_DIR/blocks $TARGET_DIR/chainstate
