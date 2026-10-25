.PHONY: all build test test-unit test-integration test-e2e test-docker test-frontend test-all lint clean help

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

clean:
	rm -rf bin/ coverage.out
