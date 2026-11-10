#!/usr/bin/env bash
set -euo pipefail

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:---compose}"

case "$mode" in
  --local)
    cd "$module_dir"
    echo "Running FraudGuard tests..."
    GOSUMDB=sum.golang.org go test ./...
    echo
    echo "Running the synthetic observe-only detector demo..."
    GOSUMDB=sum.golang.org go run ./cmd/demo
    ;;
  --compose)
    cd "$module_dir"
    echo "Running tests before the pinned Bifrost container walkthrough..."
    GOSUMDB=sum.golang.org go test ./...
    echo
    echo "Starting local Bifrost + Redis + synthetic provider smoke demo..."
    bash scripts/run-compose-demo.sh --smoke
    ;;
  --help|-h)
    echo "Usage: $0 [--compose|--local]"
    echo "  --compose  Run tests, build the disposable pinned Bifrost stack, send synthetic requests, and show local detector output (default)."
    echo "  --local    Run module tests and the synthetic detector demo without Docker."
    exit 0
    ;;
  *)
    echo "Usage: $0 [--compose|--local]" >&2
    exit 2
    ;;
esac
