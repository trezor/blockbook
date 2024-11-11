#!/usr/bin/env sh

set -e

HERE=$(dirname $(readlink -f $0))
ROOT_DIR=$(dirname $HERE)
BLOCKBOOK_DATA_DIR=$ROOT_DIR/data

echo "Removing blockbook data dir - $BLOCKBOOK_DATA_DIR..."
rm -rf $BLOCKBOOK_DATA_DIR
