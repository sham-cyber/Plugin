#!/usr/bin/env bash
set -euo pipefail

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
mode="${1:-verify}"

usage() {
  cat <<'USAGE'
Usage:
  bash scripts/setup_dev.sh [verify|--compose-demo|--help]

verify (default) checks Go and runs local build, tests, race, vet, and demo.
--compose-demo explicitly builds and runs the disposable pinned-Bifrost smoke demo.
The script does not create .env files, print secrets, or pull containers in verify mode.
USAGE
}

case "$mode" in
  --help|-h)
    usage
    exit 0
    ;;
  verify|--verify)
    command -v go >/dev/null || {
      echo "Go is required; install the version specified by go.mod/toolchain." >&2
      exit 1
    }
    go_version="$(cd "$module_dir" && GOSUMDB=sum.golang.org go env GOVERSION)" || {
      echo "Go is installed but its configured toolchain could not be verified." >&2
      echo "Retry with checksum verification enabled (GOSUMDB=sum.golang.org)." >&2
      exit 1
    }
    echo "Module Go toolchain: $go_version"
    if command -v docker >/dev/null && docker compose version >/dev/null 2>&1; then
      echo "Docker Compose is available for the optional demo."
    else
      echo "Docker Compose not found; the optional container demo will be unavailable."
    fi
    echo "Running local verification. No .env file or secret values are read or written."
    cd "$module_dir"
    make verify
    ;;
  --compose-demo)
    command -v git >/dev/null || {
      echo "git is required for the pinned-source demo." >&2
      exit 1
    }
    command -v docker >/dev/null && docker compose version >/dev/null 2>&1 || {
      echo "Docker with the Compose plugin is required for --compose-demo." >&2
      exit 1
    }
    exec bash "$module_dir/scripts/run-compose-demo.sh" --smoke
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
