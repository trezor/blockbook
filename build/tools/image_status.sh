#!/usr/bin/env bash
set -e

if [ $# -ne 2 ]; then
    echo "Invalid parameters" 1>&2
    exit 1
fi

IMG=$1
DIR=$2

IMG_CREATED_TIME=$(docker inspect --format='{{json .Metadata.LastTagTime}}' $IMG 2>/dev/null | tr -d '"')

if [ -z "$IMG_CREATED_TIME" ]; then
    echo "missing"
    exit 0
fi

IMG_CREATED_TS=$(date -d $IMG_CREATED_TIME +%s)
GIT_COMMIT_TS=$(date -d $(git log --pretty="format:%cI" -1 $DIR) +%s)

if [ $IMG_CREATED_TS -lt $GIT_COMMIT_TS ]; then
    echo "out-of-time"
else
    echo "ok"
fi

exit 0
