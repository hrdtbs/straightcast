#!/bin/sh
# dist/ にアーカイブを出す。
# 使い方: scripts/package.sh v0.1.0
set -eu

version="${1:-dev}"
outdir="${2:-dist}"
root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)

cd "$root"
rm -rf "$outdir"
mkdir -p "$outdir"

for pair in windows/amd64 linux/amd64 darwin/amd64 darwin/arm64; do
  goos=${pair%/*}
  goarch=${pair#*/}
  binary=straightcast
  if [ "$goos" = "windows" ]; then
    binary=straightcast.exe
  fi
  stage="$outdir/stage"
  mkdir -p "$stage"
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build \
    -trimpath \
    -ldflags "-s -w -X main.version=${version}" \
    -o "$stage/$binary" \
    ./cmd/straightcast
  archive="straightcast_${goos}_${goarch}"
  if [ "$goos" = "windows" ]; then
    (cd "$stage" && zip -q "../${archive}.zip" "$binary")
  else
    tar -C "$stage" -czf "$outdir/${archive}.tar.gz" "$binary"
  fi
  rm -rf "$stage"
  echo "$outdir/$archive"
done
