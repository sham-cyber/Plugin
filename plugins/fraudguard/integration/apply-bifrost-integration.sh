#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" != "--apply" || $# -ne 1 ]]; then
  echo "Usage: $0 --apply (applies changes only to a local Bifrost checkout)" >&2
  exit 2
fi

: "${BIFROST_ROOT:?Set BIFROST_ROOT to a local Bifrost checkout}"
root="$(git -C "$BIFROST_ROOT" rev-parse --show-toplevel)"
expected_commit="54ad3abec4fbbdaedf59414415094760fb9aa7e9"
actual_commit="$(git -C "$root" rev-parse HEAD)"
if [[ "$actual_commit" != "$expected_commit" ]]; then
  echo "Expected Bifrost commit $expected_commit; found $actual_commit. No changes applied." >&2
  exit 1
fi

source_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
target_dir="$root/plugins/fraudguard"
patch_file="$source_dir/integration/bifrost-54ad3ab.patch"

apply_loader_patch=0
if git -C "$root" apply --check --unidiff-zero "$patch_file"; then
  apply_loader_patch=1
elif git -C "$root" apply --reverse --check --unidiff-zero "$patch_file"; then
  echo "Bifrost loader patch is already applied."
else
  echo "The integration patch does not match this checkout. No changes applied." >&2
  exit 1
fi

if [[ -e "$target_dir" ]]; then
  if ! diff -qr "$source_dir" "$target_dir" >/dev/null; then
    echo "$target_dir already exists and differs from this module; refusing to overwrite it." >&2
    exit 1
  fi
  echo "FraudGuard module already matches the source."
else
  mkdir -p "$root/plugins"
  cp -a "$source_dir" "$target_dir"
fi

if [[ "$apply_loader_patch" -eq 1 ]]; then
  git -C "$root" apply --unidiff-zero "$patch_file"
fi

(
  cd "$root/transports"
  GOSUMDB=sum.golang.org go mod edit \
    -replace=github.com/maximhq/bifrost/core=../core \
    -replace=github.com/maximhq/bifrost/framework=../framework \
    -replace=github.com/maximhq/bifrost/plugins/compat=../plugins/compat \
    -replace=github.com/maximhq/bifrost/plugins/governance=../plugins/governance \
    -replace=github.com/maximhq/bifrost/plugins/logging=../plugins/logging \
    -replace=github.com/maximhq/bifrost/plugins/maxim=../plugins/maxim \
    -replace=github.com/maximhq/bifrost/plugins/modelcatalogresolver=../plugins/modelcatalogresolver \
    -replace=github.com/maximhq/bifrost/plugins/otel=../plugins/otel \
    -replace=github.com/maximhq/bifrost/plugins/prompts=../plugins/prompts \
    -replace=github.com/maximhq/bifrost/plugins/routing=../plugins/routing \
    -replace=github.com/maximhq/bifrost/plugins/semanticcache=../plugins/semanticcache \
    -replace=github.com/maximhq/bifrost/plugins/telemetry=../plugins/telemetry \
    -require=github.com/maximhq/bifrost/plugins/fraudguard@v0.0.0 \
    -replace=github.com/maximhq/bifrost/plugins/fraudguard=../plugins/fraudguard
)

gofmt -w \
  "$root/transports/bifrost-http/lib/config.go" \
  "$root/transports/bifrost-http/server/plugins.go" \
  "$root/transports/bifrost-http/server/fraudguard_plugin_test.go"

echo "FraudGuard is wired into this local Bifrost checkout."
echo "This script does not commit or push upstream changes."
echo "Next: cd \"$root/transports\" && GOSUMDB=sum.golang.org go test ./bifrost-http/server"
