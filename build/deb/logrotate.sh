#!/bin/bash
set -e

LOGS=$(readlink -f $(dirname $0)/../logs)

find $LOGS -mtime +30 -type f -print0 | while read -r -d $'\0' log; do
    # remove log if isn't opened by any process
    if ! fuser -s $log; then
        rm -f $log
    fi
done
