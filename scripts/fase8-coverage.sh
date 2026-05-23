#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"
OUT_DIR="${COVERAGE_OUT_DIR:-$ROOT_DIR/dogfood-output/fase8}"
PROFILE="$OUT_DIR/coverage.out"
SUMMARY="$OUT_DIR/coverage-summary.txt"

mkdir -p "$OUT_DIR"
cd "$BACKEND_DIR"

echo "[fase8-coverage] go test with coverage"
go test ./... -coverprofile="$PROFILE" -covermode=atomic

echo "[fase8-coverage] summarize package coverage"
go tool cover -func="$PROFILE" | tee "$SUMMARY"

echo "[fase8-coverage] report: $SUMMARY"
