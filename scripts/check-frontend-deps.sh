#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
FRONTEND_DIR="${ROOT_DIR}/frontend"

cd "${FRONTEND_DIR}"

echo "=================================================="
echo "Frontend Dependency Health & Freshness Policy Check"
echo "=================================================="

MODE="${1:-full}"

if [ "${MODE}" != "--freshness-only" ]; then
  echo "--> Verifying frontend lockfile consistency (npm ci)..."
  npm ci
  git diff --exit-code package.json package-lock.json

  echo "--> Running blocking frontend security audit (npm audit)..."
  npm audit --audit-level=high
fi

echo "--> Enforcing frontend dependency freshness policy..."
node "${SCRIPT_DIR}/check-frontend-freshness.cjs"
