.PHONY: all build test test-unit test-integration test-e2e test-docker test-frontend test-all lint deps deps-health deps-verify deps-audit deps-freshness clean help

# Default target runs all offline tests
all: test-unit

## Help
help:
	@echo "Kerob Automation Engine - Make Targets"
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
	@echo "Build & Lint:"
	@echo "  make build             Compile kerob server binary (CGO_ENABLED=0)"
	@echo "  make lint              Run code formatting and linter checks"
	@echo "  make clean             Clean build artifacts"
	@echo ""
	@echo "Dependency Health & Security Auditing:"
	@echo "  make deps              Run complete enforced dependency health audit (Go + Frontend)"
	@echo "  make deps-verify       Verify Go module & npm lockfile integrity and sync"
	@echo "  make deps-audit        Run blocking vulnerability scans (govulncheck + npm audit)"
	@echo "  make deps-freshness    Enforce Go and frontend dependency freshness policies"

## Build
build:
	CGO_ENABLED=0 go build -v -ldflags="-w -s" -o bin/kerob ./cmd/server

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

test-all: test-unit test-integration test-e2e test-frontend

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
	rm -rf bin/ coverage.out
