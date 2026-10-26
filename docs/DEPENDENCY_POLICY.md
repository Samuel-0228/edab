# Dependency Health & Freshness Policy

Kerob maintains an **enforced, reproducible dependency maintenance process** across both the Go backend and Svelte frontend ecosystems. Dependency security and freshness are strictly gated in continuous integration (CI) and executable via local reproducible commands.

---

## 1. Core Principles

1. **Zero Known Vulnerabilities**: Blocking security scans run on every PR and push. No vulnerabilities with high/critical severity or reachable vulnerable call stacks are permitted.
2. **Deterministic Integrity & Synchronization**: `go.sum` and `package-lock.json` must remain 100% synchronized with their manifest definitions. Any lockfile drift or untidy state fails immediately.
3. **Enforced Freshness Policies**: Dependency freshness is enforced via active policy verification scripts, not merely informational reports. Builds fail when dependencies require attention under the defined policy.
4. **Deliberate Framework Compatibility**: Upgrades are evaluated deliberately against framework boundaries. Pinned major tracks (e.g. Svelte 5 and Vite 6) are explicitly declared in `.dependency-policy.json` to prevent breaking changes.
5. **No Informational Bypasses**: Never use `|| true`, `|| [ $? -le 1 ]`, or warning-only bypasses for dependency security or freshness gating.

---

## 2. Go Dependencies

### Direct Dependencies

The Go backend adheres to minimalist dependency footprint. All direct dependencies are declared in `go.mod`:

- `github.com/google/uuid`: UUID generation for execution tracking and idempotency keys.
- `github.com/lib/pq`: PostgreSQL driver for enterprise persistence.
- `github.com/go-telegram-bot-api/telegram-bot-api/v5`: Telegram Bot API integration client.

Zero unused direct dependencies and zero untracked indirect dependencies are permitted.

### Enforced Checks

1. **Integrity**: `go mod verify` validates cryptographic hashes against the local Go module cache.
2. **Synchronization**: `go mod tidy` followed by `git diff --exit-code go.mod go.sum` guarantees that `go.mod` and `go.sum` are clean and synchronized.
3. **Vulnerability Scanning**: `govulncheck ./...` runs as a blocking check. It scans the call graph of all symbols in the application for known vulnerabilities from [vuln.go.dev](https://vuln.go.dev).
4. **Freshness Enforcement**: `scripts/check-go-deps.sh` (backed by `scripts/check-go-freshness.cjs`) parses `go list -u -m -json all`. If any direct dependency has an unapproved newer version, the check fails with exit code 1.

---

## 3. Frontend Dependencies

### Structure & Framework Stack

The control dashboard in `frontend/` utilizes Svelte 5 + Vite 6 + TypeScript 5.

### Enforced Checks

1. **Lockfile Consistency**: `npm ci` strictly validates that `package-lock.json` matches `package.json` without any mutations.
2. **Security Audit**: `npm audit --audit-level=high` runs in blocking mode to fail builds on vulnerabilities.
3. **Semver Drift Prevention**: `scripts/check-frontend-freshness.cjs` verifies that `current === wanted` for all installed packages. If an in-range patch or minor release is available in `package.json` but missing from `package-lock.json`, the check fails.
4. **Compatibility Track Enforcement**: Major version boundaries are tracked in `.dependency-policy.json`:
   - `vite` (`6.x`): Required by `@sveltejs/vite-plugin-svelte` v5 and singlefile bundling.
   - `@sveltejs/vite-plugin-svelte` (`5.x`): Svelte 5 compiler bridge.
   - `typescript` (`5.x`): Compatible with Svelte 5 language tools and ESLint 10 parser.
   - `jsdom` (`26.x`): Vitest 5 DOM simulation environment.

Any package found outdated outside of approved compatibility tracks fails CI with exit code 1.

---

## 4. Reproducible Commands

All dependency checks can be run locally using Make or standalone scripts:

| Target / Command | Scope | Description |
| :--- | :--- | :--- |
| `make deps` / `make deps-health` | Full | Runs complete Go + Frontend verification, audits, and freshness checks |
| `make deps-verify` | Integrity | Runs `go mod verify`, `go mod tidy` diff, and `npm ci` lockfile diff |
| `make deps-audit` | Security | Runs blocking `govulncheck ./...` and `npm audit --audit-level=high` |
| `make deps-freshness` | Freshness | Enforces Go and frontend dependency freshness policies |
| `./scripts/check-dependencies.sh` | Full | Standalone runner for CI and container environments |
| `cd frontend && npm run check:deps` | Frontend | Runs frontend security audit and freshness check |

---

## 5. Automated Dependabot Cadence

Dependabot is configured in `.github/dependabot.yml` to deliver weekly dependency pull requests:

- `gomod` (`/`): Weekly Go module updates.
- `npm` (`/frontend`): Weekly frontend npm package updates.
- `docker` (`/`): Monthly base image updates.
- `github-actions` (`/`): Monthly workflow action updates.

Each PR automatically triggers the blocking `dependency-health` CI pipeline.
