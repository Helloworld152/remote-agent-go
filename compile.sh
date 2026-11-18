#!/usr/bin/env bash
set -e

cd "$(dirname "$0")"
mkdir -p bin
go build -buildvcs=false -o bin/remote-agent-go .
