#!/bin/sh
# Builds one self-contained program per operating system into ./dist. No C toolchain needed.
set -e
cd "$(dirname "$0")"
rm -rf dist && mkdir dist
for t in darwin/arm64 darwin/amd64 windows/amd64 linux/amd64 linux/arm64; do
  os=${t%/*}; arch=${t#*/}; ext=""; [ "$os" = windows ] && ext=".exe"
  out="dist/socratean-$os-$arch$ext"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags "-s -w" -o "$out" .
  echo "built $out ($(du -h "$out" | cut -f1))"
done
