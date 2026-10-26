#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

echo "=================================================="
echo "Go Dependency Health & Freshness Policy Check"
echo "=================================================="

# Ensure govulncheck is found if installed in standard paths
if ! command -v govulncheck &> /dev/null; then
  if [ -x "${HOME}/go/bin/govulncheck" ]; then
    export PATH="${PATH}:${HOME}/go/bin"
  elif [ -x "${ROOT_DIR}/bin/govulncheck" ]; then
    export PATH="${PATH}:${ROOT_DIR}/bin"
  fi
fi

MODE="${1:-full}"

if [ "${MODE}" != "--freshness-only" ]; then
  echo "--> Verifying Go module integrity (go mod verify)..."
  go mod verify

  echo "--> Verifying Go module tidy synchronization..."
  go mod tidy
  git diff --exit-code go.mod go.sum

  echo "--> Running blocking vulnerability scan (govulncheck ./...)..."
  if command -v govulncheck &> /dev/null; then
    govulncheck ./...
  else
    echo "ERROR: govulncheck is not installed. Install via: go install golang.org/x/vuln/cmd/govulncheck@latest"
    exit 1
  fi
fi

echo "--> Enforcing Go dependency freshness policy..."
node "${SCRIPT_DIR}/check-go-freshness.cjs"
