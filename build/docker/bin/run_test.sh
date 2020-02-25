#!/usr/bin/env bash

for i in $(find . -name go.mod -type f -print | grep -v "^./vendor"); do
  module=$(dirname ${i})
  echo ${module}
  (cd ${module} && go test $(TAGS) ./.. $(ARGS))
done