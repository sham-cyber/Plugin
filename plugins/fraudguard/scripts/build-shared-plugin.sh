#!/usr/bin/env bash
set -euo pipefail

: "${BIFROST_ROOT:?Set BIFROST_ROOT to a prepared Bifrost checkout}"
root="$(git -C "$BIFROST_ROOT" rev-parse --show-toplevel)"
expected_commit="54ad3abec4fbbdaedf59414415094760fb9aa7e9"
actual_commit="$(git -C "$root" rev-parse HEAD)"
if [[ "$actual_commit" != "$expected_commit" ]]; then
  echo "Expected Bifrost commit $expected_commit; found $actual_commit." >&2
  exit 1
fi

module_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
workspace_root="$(cd "$module_dir/../.." && pwd)"
output="${FRAUDGUARD_PLUGIN_OUTPUT:-$workspace_root/generated-artifacts/fraudguard.so}"
if [[ ! -f "$root/plugins/fraudguard/dynamic/main.go" ]]; then
  echo "FraudGuard is not prepared in this checkout. Run prepare-pinned-bifrost-dynamic.sh --prepare first." >&2
  exit 1
fi
if ! grep -Eq 'replace[[:space:]]+github.com/maximhq/bifrost/plugins/fraudguard[[:space:]]*=>[[:space:]]*\.\./plugins/fraudguard' "$root/transports/go.mod"; then
  echo "The local FraudGuard module replacement is missing; prepare the checkout first." >&2
  exit 1
fi

mkdir -p "$(dirname "$output")"
export GOSUMDB=sum.golang.org
export CGO_ENABLED=1
(
  cd "$root/transports"
  toolchain="$(go env GOVERSION)"
  if [[ "$toolchain" != "go1.27.3" ]]; then
    echo "Bifrost requires Go 1.27.3; selected $toolchain." >&2
    exit 1
  fi
  go build -buildmode=plugin -a -trimpath -o "$output" \
    github.com/maximhq/bifrost/plugins/fraudguard/dynamic

  # Exercise the exact Bifrost shared-object loader against this built plugin.
  helper="$(mktemp "$workspace_root/generated-artifacts/fraudguard-loader-check.XXXXXX.go")"
  trap 'rm -f "$helper"' EXIT
  cat >"$helper" <<'EOF'
package main

import (
	"fmt"
	"os"

	"github.com/maximhq/bifrost/core/schemas"
	"github.com/maximhq/bifrost/framework/plugins"
)

func main() {
	if len(os.Args) != 2 {
		panic("usage: loader-check <plugin.so>")
	}
	loader := plugins.NewSharedObjectPluginLoader(nil)
	oldWrapper, err := loader.LoadPlugin(os.Args[1], map[string]any{
		"alert_threshold": float64(60),
		"warmup_requests": float64(1),
	})
	if err != nil {
		panic(fmt.Errorf("load FraudGuard .so: %w", err))
	}
	reloadedWrapper, err := loader.LoadPlugin(os.Args[1], map[string]any{
		"alert_threshold": float64(60),
		"warmup_requests": float64(2),
	})
	if err != nil {
		panic(fmt.Errorf("reload FraudGuard .so: %w", err))
	}
	if err := oldWrapper.Cleanup(); err != nil {
		panic(fmt.Errorf("cleanup replaced FraudGuard wrapper: %w", err))
	}
	defer func() {
		if err := reloadedWrapper.Cleanup(); err != nil {
			panic(fmt.Errorf("cleanup FraudGuard: %w", err))
		}
	}()
	if reloadedWrapper.GetName() != "fraudguard" {
		panic(fmt.Errorf("unexpected plugin name %q", reloadedWrapper.GetName()))
	}
	llmPlugin := plugins.AsLLMPlugin(reloadedWrapper)
	if llmPlugin == nil {
		panic("loaded shared object did not expose LLM hooks")
	}
	req := &schemas.BifrostRequest{}
	got, shortCircuit, err := llmPlugin.PreLLMHook(nil, req)
	if err != nil {
		panic(fmt.Errorf("observe-only hook failed: %w", err))
	}
	if got != req || shortCircuit != nil {
		panic("observe-only hook changed or short-circuited the request")
	}
	fmt.Println("Bifrost shared-object loader accepted FraudGuard; reload cleanup and LLM hook passed through.")
}
EOF
  go run -a -trimpath "$helper" "$output"
)

echo "Built and loader-checked FraudGuard shared object: $output"
