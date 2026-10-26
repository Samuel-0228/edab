#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

echo "=================================================="
echo "Kerob Platform - Complete Dependency Health Audit"
echo "=================================================="

"${SCRIPT_DIR}/check-go-deps.sh" "$@"
echo ""
"${SCRIPT_DIR}/check-frontend-deps.sh" "$@"

echo ""
echo "=================================================="
echo "✅ All Go and Frontend dependency checks passed!"
echo "=================================================="
