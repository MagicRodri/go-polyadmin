#!/bin/sh
# Builds, vets and tests every Go module in the repository: the core, each
# contrib adapter and the example app. `go test ./...` in one module does
# not reach the others.
set -eu
cd "$(dirname "$0")/.."
for dir in . contrib/fiber contrib/gorm examples/fiber; do
	echo "== $dir"
	(cd "$dir" && go build ./... && go vet ./... && go test ./...)
done
