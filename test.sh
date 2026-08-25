#!/bin/bash

if [ "$1" = "bench" ]; then
    go test ./... -bench=.
    exit $?
fi

go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
