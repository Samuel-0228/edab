#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

cd "${ROOT_DIR}"

VERSION="${VERSION:-$(grep -E '^## \[v[0-9]' CHANGELOG.md | head -n 1 | sed -E 's/.*\[(v[0-9]+\.[0-9]+\.[0-9]+)\].*/\1/')}"
COMMIT="${COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")}"
BUILD_DATE="${BUILD_DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

DIST_DIR="${ROOT_DIR}/dist"
BIN_DIR="${DIST_DIR}/bin"

echo "=================================================="
echo "Kerob Automation Engine - Release Artifact Builder"
echo "=================================================="
echo "Version:    ${VERSION}"
echo "Commit:     ${COMMIT}"
echo "Build Date: ${BUILD_DATE}"
echo "Dist Dir:   ${DIST_DIR}"
echo "=================================================="

rm -rf "${DIST_DIR}"
mkdir -p "${BIN_DIR}"

LDFLAGS="-w -s -X kerob/internal/version.Version=${VERSION} -X kerob/internal/version.Commit=${COMMIT} -X kerob/internal/version.BuildDate=${BUILD_DATE}"

# Target platforms: OS/ARCH
TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
  "windows/amd64"
)

# 1. Ensure frontend dashboard is built and embedded
if [ ! -f "internal/api/dashboard.html" ] || [ ! -f "internal/api/kerob_logo.png" ]; then
  echo "--> Embedded dashboard assets missing. Building frontend..."
  (cd frontend && npm ci && npm run build)
fi

echo "--> Compiling release binaries across supported platforms..."

for TARGET in "${TARGETS[@]}"; do
  GOOS="${TARGET%%/*}"
  GOARCH="${TARGET##*/}"
  
  BINARY_NAME="kerob"
  if [ "${GOOS}" = "windows" ]; then
    BINARY_NAME="kerob.exe"
  fi
  
  OUTPUT_DIR="${BIN_DIR}/${GOOS}_${GOARCH}"
  mkdir -p "${OUTPUT_DIR}"
  
  echo "    Building ${GOOS}/${GOARCH}..."
  CGO_ENABLED=0 GOOS="${GOOS}" GOARCH="${GOARCH}" go build \
    -trimpath \
    -ldflags="${LDFLAGS}" \
    -o "${OUTPUT_DIR}/${BINARY_NAME}" \
    ./cmd/server
done

echo "--> Packaging archives (tar.gz / zip)..."

for TARGET in "${TARGETS[@]}"; do
  GOOS="${TARGET%%/*}"
  GOARCH="${TARGET##*/}"
  
  SOURCE_DIR="${BIN_DIR}/${GOOS}_${GOARCH}"
  ARCHIVE_NAME="kerob-${VERSION}-${GOOS}-${GOARCH}"
  STAGE_DIR="${DIST_DIR}/${ARCHIVE_NAME}"
  
  mkdir -p "${STAGE_DIR}"
  cp "${SOURCE_DIR}/"* "${STAGE_DIR}/"
  cp "${ROOT_DIR}/README.md" "${STAGE_DIR}/"
  cp "${ROOT_DIR}/LICENSE" "${STAGE_DIR}/"
  cp "${ROOT_DIR}/.env.example" "${STAGE_DIR}/"
  
  if [ "${GOOS}" = "windows" ]; then
    (cd "${DIST_DIR}" && zip -q -r "${ARCHIVE_NAME}.zip" "${ARCHIVE_NAME}")
    echo "    Created ${ARCHIVE_NAME}.zip"
  else
    (cd "${DIST_DIR}" && tar -czf "${ARCHIVE_NAME}.tar.gz" "${ARCHIVE_NAME}")
    echo "    Created ${ARCHIVE_NAME}.tar.gz"
  fi
  
  rm -rf "${STAGE_DIR}"
done

echo "--> Generating SHA-256 checksums..."
(
  cd "${DIST_DIR}"
  if command -v sha256sum &> /dev/null; then
    sha256sum kerob-* > checksums.txt
  elif command -v shasum &> /dev/null; then
    shasum -a 256 kerob-* > checksums.txt
  fi
)

echo "=================================================="
echo "Artifacts generated successfully in ${DIST_DIR}:"
ls -lh "${DIST_DIR}"/kerob-* "${DIST_DIR}"/checksums.txt
echo "=================================================="
