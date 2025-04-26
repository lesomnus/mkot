#!/usr/bin/env bash

set -o errexit
set -o pipefail
set -o nounset
# set -o xtrace

__dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)" # Directory where this script exists.
__root="$(cd "$(dirname "${__dir}")" && pwd)"         # Root directory of project.



cd "$__root"
echo "[INFO] Generating go.work in $__root"

if [ -f "$__root/go.work" ]; then
  echo "[INFO] Removing existing go.work"
  rm "$__root/go.work"
fi

go work init
go work use .

# exporters/* 하위 모듈 추가
for dir in exporters/*; do
  if [ -d "$dir" ] && [ -f "$dir/go.mod" ]; then
    echo "[INFO] Adding module: $dir"
    go work use "$dir"
  fi
done

echo "[INFO] go.work file has been generated"
