#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CACHE_DIR="${HOME}/.cache/homectl"
mkdir -p "${CACHE_DIR}/gocache" "${CACHE_DIR}/gopath" "${CACHE_DIR}/golangci-lint"

GO_IMAGE="golang:1.27.1"
LINT_IMAGE="golangci/golangci-lint:latest"

DOCKER_RUN_GO="docker run --rm -u $(id -u):$(id -g) \
  -v ${ROOT_DIR}:/app -w /app \
  -v ${CACHE_DIR}/gocache:/cache/go-build \
  -v ${CACHE_DIR}/gopath:/cache/go-mod \
  -e GOCACHE=/cache/go-build \
  -e GOPATH=/cache/go-mod \
  -e HOME=/tmp \
  ${GO_IMAGE}"

DOCKER_RUN_LINT="docker run --rm -u $(id -u):$(id -g) \
  -v ${ROOT_DIR}:/app -w /app \
  -v ${CACHE_DIR}/gocache:/cache/go-build \
  -v ${CACHE_DIR}/gopath:/cache/go-mod \
  -v ${CACHE_DIR}/golangci-lint:/cache/golangci-lint \
  -e GOCACHE=/cache/go-build \
  -e GOPATH=/cache/go-mod \
  -e GOLANGCI_LINT_CACHE=/cache/golangci-lint \
  -e HOME=/tmp \
  ${LINT_IMAGE}"

echo "==> Step 1: gofmt check"
UNFORMATTED=$(${DOCKER_RUN_GO} sh -c 'FILES=$(find . -name "*.go" -not -path "./.git/*"); if [ -n "$FILES" ]; then gofmt -l $FILES; fi' || true)
if [ -n "$UNFORMATTED" ]; then
  echo "Error: Unformatted Go files found:"
  echo "$UNFORMATTED"
  exit 1
fi
echo "gofmt passed."

echo "==> Step 2: go vet"
${DOCKER_RUN_GO} go vet ./...
echo "go vet passed."

echo "==> Step 3: golangci-lint"
${DOCKER_RUN_LINT} golangci-lint run ./...
echo "golangci-lint passed."

echo "==> Step 4: go test -race with coverage"
${DOCKER_RUN_GO} go test -race -coverprofile=coverage.out ./... | tee coverage.pkgs.txt
echo "go test passed."

echo "==> Step 5: coverage threshold check (>= 80% aggregate and per package, excluding cmd/)"
${DOCKER_RUN_GO} sh -c '
  grep -v "github.com/px1/home-ctl/cmd/" coverage.out > coverage_prod.out || true
  if [ ! -s coverage_prod.out ] || [ $(wc -l < coverage_prod.out) -le 1 ]; then
    echo "No production package coverage data."
    exit 0
  fi
  TOTAL=$(go tool cover -func=coverage_prod.out | grep "total:" | awk "{print \$3}" | tr -d "%")
  echo "Aggregate production coverage: ${TOTAL}%"
  awk -v total="$TOTAL" '\''BEGIN { if (total + 0.0 < 80.0) { printf "Coverage %.1f%% is below required 80.0%%\n", total; exit 1 } else { printf "Coverage check passed (%.1f%% >= 80.0%%)\n", total } }'\''
'

# Per-package gate: every production package (outside cmd/) must reach 80% on its own.
# Packages without statements (types/interfaces only) report "[no statements]" and are skipped.
awk '
  /^\?/ && $2 !~ /\/cmd\// { printf "FAIL: %s has no test files\n", $2; bad = 1; next }
  /^ok/ && $2 !~ /\/cmd\// {
    if ($0 ~ /\[no statements\]/) next
    if (match($0, /coverage: [0-9.]+%/)) {
      pct = substr($0, RSTART + 10, RLENGTH - 11) + 0
      if (pct < 80.0) { printf "FAIL: %s coverage %.1f%% is below 80.0%%\n", $2, pct; bad = 1 }
    }
  }
  END { if (bad) exit 1; print "Per-package coverage check passed (every production package >= 80.0%)" }
' coverage.pkgs.txt
