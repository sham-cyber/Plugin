#!/usr/bin/env bash
set -euo pipefail

: "${BIFROST_ROOT:?Set BIFROST_ROOT to a Bifrost checkout at the pinned commit}"

expected_commit="54ad3abec4fbbdaedf59414415094760fb9aa7e9"
actual_commit="$(git -C "$BIFROST_ROOT" rev-parse HEAD)"
if [[ "$actual_commit" != "$expected_commit" ]]; then
  printf 'Expected Bifrost commit %s, found %s\n' "$expected_commit" "$actual_commit" >&2
  exit 1
fi
if [[ ! -f "$BIFROST_ROOT/core/go.mod" ]]; then
  printf 'Could not find core/go.mod under BIFROST_ROOT=%s\n' "$BIFROST_ROOT" >&2
  exit 1
fi

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
work_dir="$(mktemp -d "${TMPDIR:-/tmp}/fraudguard-verify.XXXXXX")"
trap 'rm -rf "$work_dir"' EXIT

export GOSUMDB=sum.golang.org
(
  cd "$work_dir"
  go work init "$module_dir" "$BIFROST_ROOT/core"
)
export GOWORK="$work_dir/go.work"

cd "$module_dir"
go test ./...
go test -race ./...
go vet ./...
go run ./cmd/demo
