# Kerob Developer Tooling, SDKs, and GitOps Guide

This guide covers the **Kerob CLI**, the **Go and TypeScript SDKs**, and the **GitOps workflow deployment system**.

---

## 1. Kerob CLI (`kerob` / `kerob-cli`)

The Kerob CLI provides developer and DevOps operations for local development, CI/CD pipelines, and runtime management.

### Installation & Building

```bash
# Build the unified Kerob binary (server + CLI)
make build
# Output: bin/kerob

# Or build the dedicated standalone CLI binary
make build-cli
# Output: bin/kerob-cli
```

### Global Configuration & Authentication

The CLI persists its configuration in `~/.kerob/config.json` (with strict `0600` permissions):

```bash
# Login and store API credentials
kerob login --endpoint https://kerob.internal:8080 --token krb_sec_xxxx

# Select active tenant context
kerob tenant list
kerob tenant select ten_production
kerob tenant current
```

Environment variables can also be used without writing to disk (ideal for CI/CD runners):
- `KEROB_ENDPOINT`: API server base URL (default: `http://localhost:8080`)
- `KEROB_TOKEN`: API Bearer authentication token
- `KEROB_TENANT_ID`: Organizational tenant ID

### Global Flags

Every command supports:
- `--json`: Enables script-friendly structured JSON output (ideal for `jq` and automated tooling)
- `--endpoint <url>`: Overrides target server endpoint
- `--token <token>`: Overrides authentication token
- `--tenant <id>`: Overrides tenant context

---

## 2. Workflow Management & GitOps Deployment

### 2.1 Static Validation & Secret Safety Scanning

Before any workflow is deployed or committed to Git, the CLI validates its structure, control flow, and secret safety:

```bash
kerob workflow validate ./workflows/order_fulfillment.yaml
```

#### Secret Safety Enforcements
> [!IMPORTANT]
> Plaintext secrets (API keys, AWS keys, GitHub tokens, private keys, passwords) **must never be committed** to workflow definitions.
> Secrets must be referenced by name via `credential_id` or template interpolation:
> ```yaml
> # INCORRECT (Rejected by validator):
> config:
>   api_key: "AKIAIOSFODNN7EXAMPLE"
>
> # CORRECT (Approved by validator):
> credential_id: "aws_prod_credentials"
> # OR:
> config:
>   api_key: "{{ secret.AWS_ACCESS_KEY_ID }}"
> ```

### 2.2 GitOps Deployment (`kerob deploy`)

The `deploy` command enables declarative, Git-driven workflow management:

```bash
# Dry-run: preview additions and modifications with unified diffs
kerob deploy ./workflows --dry-run --diff

# Require approval before mutating production workflows
kerob deploy ./workflows --require-approval --approve

# Automatic rollback if any deployed workflow fails verification
kerob deploy ./workflows --rollback-on-failure
```

### 2.3 Import, Export, Versions, Rollback & Diff

```bash
# Import a workflow definition from JSON or YAML
kerob workflow import ./workflows/payment.yaml

# Export existing workflow definition
kerob workflow export wf_order_123 --format yaml --out ./workflows/payment.yaml

# Publish / enable workflow for live traffic
kerob workflow publish wf_order_123

# Inspect version history and diff between releases
kerob workflow diff wf_order_123 1 2

# Instant rollback to a previous version snapshot
kerob workflow rollback wf_order_123 1
```

---

## 3. Operations, Executions, and Sagas

### 3.1 Inspecting Execution Runs & Step Timelines

```bash
# List recent executions with optional filters
kerob execution list --status failed --workflow-id wf_order_123

# Granular step timeline inspection
kerob execution get exec_55fa6211

# Inspect step execution logs and outputs
kerob logs exec_55fa6211
```

### 3.2 Execution Lifecycle Actions

```bash
# Retry a failed execution
kerob execution retry exec_55fa6211

# Pause and resume in-flight executions
kerob execution pause exec_55fa6211
kerob execution resume exec_55fa6211

# Cancel an active execution
kerob execution cancel exec_55fa6211

# Trigger manual operator saga rollback compensation
kerob execution compensate exec_55fa6211 --reason "Customer order cancelled"
```

---

## 4. Pipeline Events and Dead-Letter Queue (DLQ)

```bash
# Replay an event through automation matching
kerob event replay ev_1001 --bypass-idempotency

# List dead-letter queue records
kerob dlq list

# Retry a dead-letter failure
kerob dlq retry dlq_2002

# Discard an unrecoverable failure
kerob dlq discard dlq_2002
```

---

## 5. Shell Auto-Completion

The CLI includes built-in auto-completion scripts for `bash`, `zsh`, and `fish`:

```bash
# Bash
source <(kerob completion bash)

# Zsh
kerob completion zsh > ~/.zsh/completion/_kerob

# Fish
kerob completion fish > ~/.config/fish/completions/kerob.fish
```

---

## 6. Programmatic SDKs

### 6.1 Go SDK (`kerob/pkg/sdk`)

Import the typed Go SDK directly into your Go services or CLI tools:

```go
package main

import (
	"context"
	"fmt"
	"log"

	"kerob/pkg/sdk"
)

func main() {
	client, err := sdk.NewClient(
		"http://localhost:8080",
		sdk.WithToken("krb_live_token"),
		sdk.WithTenant("ten_prod"),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()

	// List workflows
	workflows, err := client.ListWorkflows(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for _, wf := range workflows {
		fmt.Printf("Workflow: %s (%s, enabled: %t)\n", wf.Name, wf.ID, wf.Enabled)
	}
}
```

### 6.2 TypeScript SDK (`@kerob/sdk`)

```typescript
import { KerobClient, validateWorkflowSecrets, deployWorkflows } from '@kerob/sdk';

const client = new KerobClient({
  baseUrl: 'https://kerob.internal:8080',
  token: process.env.KEROB_TOKEN,
  tenantId: 'ten_prod'
});

// List executions
const executions = await client.listExecutions({ status: 'failed' });
console.log(`Failed executions: ${executions.length}`);

// Trigger saga compensation
if (executions.length > 0) {
  await client.compensateExecution(executions[0].id, 'Automated CI/CD recovery');
}
```

---

## 7. CI/CD Pipeline Integration (GitOps)

### GitHub Actions Workflow Example

Create `.github/workflows/kerob-deploy.yml`:

```yaml
name: Deploy Kerob Workflows (GitOps)

on:
  push:
    branches: [main]
    paths:
      - 'workflows/**'
  pull_request:
    branches: [main]
    paths:
      - 'workflows/**'

jobs:
  validate-and-deploy:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.23'

      - name: Compile Kerob CLI
        run: go build -o /usr/local/bin/kerob ./cmd/server

      # PRs: Validate syntax, secrets, and run dry-run diff
      - name: GitOps Dry-Run (Pull Request)
        if: github.event_name == 'pull_request'
        env:
          KEROB_ENDPOINT: ${{ secrets.KEROB_ENDPOINT }}
          KEROB_TOKEN: ${{ secrets.KEROB_TOKEN }}
        run: |
          kerob deploy ./workflows --dry-run --diff

      # Main branch push: Apply validated workflow deployment
      - name: GitOps Live Deployment (Main)
        if: github.event_name == 'push' && github.ref == 'refs/heads/main'
        env:
          KEROB_ENDPOINT: ${{ secrets.KEROB_ENDPOINT }}
          KEROB_TOKEN: ${{ secrets.KEROB_TOKEN }}
        run: |
          kerob deploy ./workflows --rollback-on-failure
```
