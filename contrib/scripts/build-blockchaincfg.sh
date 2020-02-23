#!/usr/bin/env bash
set -e
# Activate go modules support.
GO111MODULE='on' 

# build the blockbook binary.
go build

# generate the config files.
go run build/templates/generate.go $1 > /dev/null
mv build/pkg-defs/blockbook/blockchaincfg.json build
rm -rf build/pkg-defs
echo Generated build/blockchaincfg.json for $1
