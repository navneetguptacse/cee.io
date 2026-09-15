#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(dirname "$SCRIPT_DIR")"

echo "Building CEE binary..."
mkdir -p "$PROJECT_DIR/bin"
cd "$PROJECT_DIR"
go build -ldflags="-s -w" -o "$PROJECT_DIR/bin/cee" ./cmd/cee

echo "Built successfully: $PROJECT_DIR/bin/cee"
"$PROJECT_DIR/bin/cee" --help

