#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

CACHE_DIR="${HOME}/.cache/homectl"
mkdir -p "${CACHE_DIR}/gocache" "${CACHE_DIR}/gopath" "${CACHE_DIR}/golangci-lint"

GO_IMAGE="golang:1.27.2"
LINT_IMAGE="golangci/golangci-lint:v2.14.0-alpine"

# Functions, not command strings: arguments and paths are never word-split, and a failing
# docker run fails the calling line (set -e), so it cannot be mistaken for a clean result.
run_go() {
  docker run --rm -u "$(id -u):$(id -g)" \
    -v "${ROOT_DIR}:/app" -w /app \
    -v "${CACHE_DIR}/gocache:/cache/go-build" \
    -v "${CACHE_DIR}/gopath:/cache/go-mod" \
    -e GOCACHE=/cache/go-build \
    -e GOPATH=/cache/go-mod \
    -e HOME=/tmp \
    "${GO_IMAGE}" "$@"
}

run_lint() {
  docker run --rm -u "$(id -u):$(id -g)" \
    -v "${ROOT_DIR}:/app" -w /app \
    -v "${CACHE_DIR}/gocache:/cache/go-build" \
    -v "${CACHE_DIR}/gopath:/cache/go-mod" \
    -v "${CACHE_DIR}/golangci-lint:/cache/golangci-lint" \
    -e GOCACHE=/cache/go-build \
    -e GOPATH=/cache/go-mod \
    -e GOLANGCI_LINT_CACHE=/cache/golangci-lint \
    -e HOME=/tmp \
    "${LINT_IMAGE}" "$@"
}

echo "==> Step 1: gofmt check"
UNFORMATTED="$(run_go sh -c 'find . -name "*.go" -not -path "./.git/*" -exec gofmt -l {} +')"
if [[ -n "$UNFORMATTED" ]]; then
  echo "Error: Unformatted Go files found:" >&2
  echo "$UNFORMATTED" >&2
  exit 1
fi
echo "gofmt passed."

echo "==> Step 2: go vet"
run_go go vet ./...
echo "go vet passed."

echo "==> Step 3: golangci-lint"
run_lint golangci-lint run ./...
echo "golangci-lint passed."

echo "==> Step 4: go test -race with coverage"
run_go go test -race -coverprofile=coverage.out ./... | tee coverage.pkgs.txt
echo "go test passed."

echo "==> Step 5: coverage threshold check (>= 80% aggregate and per package, excluding cmd/)"
# This inner shell does not run with set -e, so every check below reports its own status.
run_go sh -c '
  grep -v "github.com/px1/home-ctl/cmd/" coverage.out > coverage_prod.out
  if [ "$(wc -l < coverage_prod.out)" -le 1 ]; then
    echo "No production package coverage data: the gate cannot pass without it."
    exit 1
  fi
  TOTAL=$(go tool cover -func=coverage_prod.out | awk "/^total:/ {print \$3}" | tr -d "%")
  if [ -z "$TOTAL" ]; then
    echo "Aggregate coverage not reported by go tool cover."
    exit 1
  fi
  echo "Aggregate production coverage: ${TOTAL}%"
  awk -v total="$TOTAL" '\''BEGIN { if (total + 0.0 < 80.0) { printf "Coverage %.1f%% is below required 80.0%%\n", total; exit 1 } else { printf "Coverage check passed (%.1f%% >= 80.0%%)\n", total } }'\''
'

# Per-package gate: every production package (outside cmd/) must appear in the test output and reach
# 80% on its own. Packages without statements (types/interfaces only) report "[no statements]" and are skipped.
status=0

PACKAGES="$(run_go go list ./...)"
while IFS= read -r pkg; do
  case "$pkg" in
    */cmd/*) continue ;;
    *) ;;
  esac
  if ! awk -v p="$pkg" '$2 == p { found = 1 } END { exit !found }' coverage.pkgs.txt; then
    echo "FAIL: $pkg is missing from the go test output"
    status=1
  fi
done <<< "$PACKAGES"

awk '
  /^FAIL/ { printf "FAIL: %s\n", $0; bad = 1; next }
  /^\?/ && $2 !~ /\/cmd\// { printf "FAIL: %s has no test files\n", $2; bad = 1; next }
  /^ok/ && $2 !~ /\/cmd\// {
    if ($0 ~ /\[no statements\]/) next
    if (match($0, /coverage: [0-9.]+%/)) {
      pct = substr($0, RSTART + 10, RLENGTH - 11) + 0
      if (pct < 80.0) { printf "FAIL: %s coverage %.1f%% is below 80.0%%\n", $2, pct; bad = 1 }
    }
  }
  END { if (bad) exit 1 }
' coverage.pkgs.txt || status=1

if [[ "$status" -ne 0 ]]; then
  echo "Per-package coverage check failed."
  exit 1
fi
echo "Per-package coverage check passed (every production package >= 80.0%)"
