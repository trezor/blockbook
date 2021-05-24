{{define "main" -}}
#!/usr/bin/env bash
set -e

LOGS={{.Env.BlockbookInstallPath}}/{{.Coin.Alias}}/logs

find $LOGS -mtime +7 -type f -print0 | while read -r -d $'\0' log; do
    # remove log if isn't opened by any process
    if ! fuser -s $log; then
        rm -f $log
    fi
done
{{end}}
