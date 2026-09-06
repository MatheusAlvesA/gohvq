#!/bin/bash
set -euo pipefail

cd -- "$(dirname -- "${BASH_SOURCE[0]}")"

if (( $# > 1 )) || [[ "${1:-}" != "" && "${1:-}" != "bench" ]]; then
    echo "Usage: $0 [bench]" >&2
    exit 2
fi

go vet ./...
go test -race -count=1 -covermode=atomic -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

if [[ "${1:-}" == "bench" ]]; then
    go test -run='^$' -bench=. -count=1 ./...
fi
