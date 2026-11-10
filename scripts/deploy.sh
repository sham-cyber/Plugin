#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
VERSION_RE='^v[0-9]+\.[0-9]+\.[0-9]+([.-][A-Za-z0-9.-]+)?$'

usage() {
  cat <<'EOF'
FraudGuard local release helper (no production deploy target is configured).

Usage:
  bash scripts/deploy.sh preflight
  bash scripts/deploy.sh package vX.Y.Z
  bash scripts/deploy.sh tag vX.Y.Z
  bash scripts/deploy.sh demo
  bash scripts/deploy.sh deploy

preflight runs local build, tests, race checks, vet, and the synthetic demo.
package creates an ignored source archive under generated-artifacts/.
tag creates a local annotated Git tag after a clean-tree preflight; it is not pushed.
demo runs the existing disposable local Docker Compose walkthrough.
deploy intentionally refuses because no production gateway/deployment target is configured.
EOF
}

validate_version() {
  local version="${1:-}"
  if [[ ! "$version" =~ $VERSION_RE ]]; then
    echo "Use a release version like v1.2.3 or v1.2.3-rc.1." >&2
    exit 2
  fi
}

preflight() {
  echo "Running local FraudGuard verification; no deployment or remote release action will run."
  git -C "$ROOT_DIR" diff --check
  git -C "$ROOT_DIR" diff --cached --check
  make -C "$ROOT_DIR" fraudguard-verify
  echo "Local preflight passed. This does not verify a production gateway, live Redis, or CI."
}

mode="${1:-help}"
case "$mode" in
  help|-h|--help)
    usage
    ;;
  preflight)
    preflight
    ;;
  package)
    validate_version "${2:-}"
    preflight
    output_dir="$ROOT_DIR/generated-artifacts"
    archive="$output_dir/fraudguard-${2}.tar.gz"
    mkdir -p "$output_dir"
    tar -czf "$archive" -C "$ROOT_DIR" \
      --exclude='.git' \
      --exclude='*/.env*' \
      --exclude='*.pem' \
      --exclude='*.key' \
      --exclude='*/node_modules' \
      --exclude='artifacts/fraudguard-bifrost-page/node_modules' \
      --exclude='artifacts/fraudguard-bifrost-page/dist' \
      Makefile LICENSE README.md PITCH.md PRESENTATION.md RELEASE.md MONETIZATION.md \
      VERIFICATION_REPORT.md scripts/deploy.sh plugins/fraudguard \
      artifacts/fraudguard-bifrost-page
    sha256sum "$archive"
    echo "Source archive created at generated-artifacts/fraudguard-${2}.tar.gz"
    ;;
  tag)
    validate_version "${2:-}"
    preflight
    if [[ -n "$(git -C "$ROOT_DIR" status --porcelain)" ]]; then
      echo "Refusing to tag: the working tree must be clean." >&2
      exit 2
    fi
    if git -C "$ROOT_DIR" rev-parse -q --verify "refs/tags/$2" >/dev/null; then
      echo "Refusing to overwrite existing tag $2." >&2
      exit 2
    fi
    git -C "$ROOT_DIR" tag -a "$2" -m "FraudGuard $2"
    echo "Created local tag $2; it was not pushed."
    ;;
  demo)
    bash "$ROOT_DIR/plugins/fraudguard/scripts/demo_walkthrough.sh" --compose
    ;;
  deploy)
    echo "No production deployment target is configured. No deploy, Docker publish, or release tag was attempted." >&2
    exit 2
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
