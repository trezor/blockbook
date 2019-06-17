#!/usr/bin/env bash
set -e
go run build/templates/generate.go $1 > /dev/null
mv build/pkg-defs/blockbook/blockchaincfg.json build
rm -rf build/pkg-defs
echo Generated build/blockchaincfg.json for $1
