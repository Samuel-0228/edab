# Contributing to Kerob

Thank you for your interest in contributing to **Kerob**, an event-driven automation platform built in Go.

---

## 1. Development Principles

1. **Clear Separation of Concerns**: Core engine logic must remain decoupled from service-specific adapters.
2. **Safe & Deterministic**: Conditions and transformations must never execute unrestricted arbitrary code (no eval, shell, or SQL interpretation).
3. **Security First**: Outbound HTTP actions must route through the SSRF protection validator. Secrets must never be committed, logged, or returned in plaintext via API endpoints.
4. **Strict Idempotency**: Duplicate events or retries must never trigger duplicate actions or posts.
5. **Production Readiness**: All changes require unit tests, integration tests, clean `go vet`, and race-detector verification (`go test -race ./...`).

---

## 2. Local Environment Setup

### Prerequisites
- Go 1.23+
- Docker and Docker Compose
- PostgreSQL 15+ (or run via `docker compose up -d postgres`)

### Quickstart
```bash
# 1. Clone repository
git clone https://github.com/Samuel-0228/edab.git
cd edab

# 2. Copy environment template
cp .env.example .env

# 3. Start local PostgreSQL
docker compose up -d postgres

# 4. Run tests
go test -v -race ./...

# 5. Start development server
go run ./cmd/server
```

---

## 3. Running Quality Checks

Before submitting a Pull Request, verify all quality gates pass:

```bash
# Check formatting
gofmt -l .

# Run static analysis
go vet ./...

# Run complete test suite with race detector
go test -v -count=1 -race ./...

# Build binary
CGO_ENABLED=0 go build -ldflags="-w -s" -o /tmp/kerob ./cmd/server
```

---

## 4. Conventional Commits

Commit messages must follow the Conventional Commits specification:

```text
feat(core): introduce condition evaluation rules
feat(adapters): add Discord embed delivery action
fix(ssrf): prevent loopback address bypass on redirect
test(api): cover invalid workflow payloads
docs(api): update OpenAPI specifications
```
