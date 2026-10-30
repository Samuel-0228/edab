.PHONY: all build build-frontend release-build release-artifacts release test test-unit test-integration test-e2e test-docker test-frontend test-coverage test-coverage-frontend coverage test-all lint deps deps-health deps-verify deps-audit deps-freshness clean help

VERSION ?= $(shell grep -E '^## \[v[0-9]' CHANGELOG.md | head -n 1 | sed -E 's/.*\[(v[0-9]+\.[0-9]+\.[0-9]+)\].*/\1/')
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -w -s -X kerob/internal/version.Version=$(VERSION) -X kerob/internal/version.Commit=$(COMMIT) -X kerob/internal/version.BuildDate=$(BUILD_DATE)

# Default target runs all offline tests
all: test-unit

## Help
help:
	@echo "Kerob Automation Engine - Make Targets"
	@echo ""
	@echo "Release & Build:"
	@echo "  make build             Compile kerob server binary with embedded dashboard and version metadata"
	@echo "  make build-frontend    Compile Svelte dashboard into internal/api embedded bundle"
	@echo "  make release-artifacts Compile multi-platform distributable binaries and archives with checksums"
	@echo "  make release           Run all verification suites and produce release artifacts"
	@echo "  make clean             Clean build and release artifacts"
	@echo ""
	@echo "Test Suites:"
	@echo "  make test-unit         Run unit tests (no external services or Docker required)"
	@echo "  make test-integration  Run integration tests (in-memory multi-component pipelines)"
	@echo "  make test-e2e          Run end-to-end lifecycle and dead-letter/retry tests"
	@echo "  make test-docker       Run Docker & Docker Compose configuration tests (skips if Docker unavailable)"
	@echo "  make test-frontend     Run frontend unit & component test suite (Vitest + JSDOM, no backend required)"
	@echo "  make test              Run standard Go test suite for fresh clone verification (go test ./...)"
	@echo "  make test-all          Run complete test verification (unit, integration, e2e, frontend)"
	@echo ""
	@echo "Lint & Verification:"
	@echo "  make lint              Run code formatting, type checking, and linter checks"
	@echo ""
	@echo "Dependency Health & Security Auditing:"
	@echo "  make deps              Run complete enforced dependency health audit (Go + Frontend)"
	@echo "  make deps-verify       Verify Go module & npm lockfile integrity and sync"
	@echo "  make deps-audit        Run blocking vulnerability scans (govulncheck + npm audit)"
	@echo "  make deps-freshness    Enforce Go and frontend dependency freshness policies"

## Build
build:
	CGO_ENABLED=0 go build -v -ldflags="$(LDFLAGS)" -o bin/kerob ./cmd/server

build-frontend:
	cd frontend && npm ci && npm run build

## Release Packaging
release-artifacts:
	./scripts/build-release.sh

release: test-all lint release-artifacts

## Tests
test:
	go test -v -race $$(go list ./... | grep -v node_modules)

test-unit:
	go test -v -race $$(go list ./... | grep -v '/tests' | grep -v node_modules)

test-integration:
	go test -v -race ./tests -run "TestAPI|TestEngine|TestTelegram|Integration"

test-e2e:
	go test -v -race ./tests -run "TestE2E|E2E"

test-docker:
	go test -v -race ./tests -run "TestDockerCompose"

test-frontend:
	cd frontend && npm test

test-coverage:
	go test -coverprofile=coverage.out $$(go list ./... | grep -v node_modules)
	go tool cover -func=coverage.out
	@coverage=$$(go tool cover -func=coverage.out | grep total: | awk '{print substr($$3, 1, length($$3)-1)}'); \
	echo "Go Total Coverage: $${coverage}% (minimum required: 60.0%)"; \
	awk -v cov="$$coverage" 'BEGIN { if (cov + 0 < 60.0) { print "FAILED: Go coverage below 60.0%"; exit 1; } else { print "PASSED: Go coverage threshold met"; exit 0; } }'

test-coverage-frontend:
	cd frontend && npm test -- --coverage

coverage: test-coverage test-coverage-frontend

test-all: test-unit test-integration test-e2e test-frontend coverage

## Lint & Format
lint:
	gofmt -l .
	cd frontend && npm run check && npm run lint

## Dependency Health & Auditing
deps: deps-health

deps-verify:
	@echo "==> Verifying Go module integrity & tidy state..."
	go mod verify
	go mod tidy
	git diff --exit-code go.mod go.sum
	@echo "==> Verifying frontend lockfile consistency..."
	cd frontend && npm ci
	git diff --exit-code frontend/package.json frontend/package-lock.json

deps-audit:
	@echo "==> Running Go vulnerability scan (govulncheck)..."
	govulncheck ./...
	@echo "==> Running frontend security audit (npm audit)..."
	cd frontend && npm audit --audit-level=high

deps-freshness:
	@echo "==> Checking Go dependency freshness policy..."
	./scripts/check-go-deps.sh --freshness-only
	@echo "==> Checking frontend dependency freshness policy..."
	./scripts/check-frontend-deps.sh --freshness-only

deps-health:
	./scripts/check-dependencies.sh

clean:
	rm -rf bin/ dist/ coverage.out coverage_api.out
