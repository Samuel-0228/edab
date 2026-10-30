# Kerob Workflow Specification & Reference Manual

Kerob is an event-driven automation platform built around the declarative execution model:
```
WHEN (Trigger) ➔ IF (Conditions) ➔ TRANSFORM (Formatting) ➔ THEN (Action Pipeline)
```

This reference documents the complete schema, data flow syntax, control flow primitives, filtering library, integrations, and reliability controls available in Kerob.

---

## 1. Workflow Architecture & Top-Level Schema

A workflow definition is a JSON or YAML object representing an automated pipeline:

```json
{
  "id": "wf_incident_response",
  "name": "Incident Escalation Pipeline",
  "description": "Filters critical pager alerts and orchestrates multi-step team notifications",
  "enabled": true,
  "trigger": {
    "type": "webhook",
    "event_types": ["pagerduty.incident"],
    "source_id": "src_pagerduty"
  },
  "conditions": {
    "field": "payload.severity",
    "operator": "equals",
    "value": "CRITICAL"
  },
  "transformations": {
    "type": "mapping",
    "field_maps": {
      "summary": "🚨 [{{payload.urgency | upper}}] {{payload.title | trim | truncate:60}}",
      "timestamp": "{{payload.created_at | date_format:\"2006-01-02 15:04:05\"}}"
    }
  },
  "actions": [
    {
      "id": "slack_alert",
      "integration": "slack",
      "action_type": "post_message",
      "config": {
        "channel": "#incident-response",
        "text": "{{transformed.summary}} — Reported at {{transformed.timestamp}}"
      },
      "on_error": "continue",
      "timeout": "10s",
      "retry": {
        "max_retries": 3,
        "backoff": "2s"
      }
    },
    {
      "id": "email_oncall",
      "integration": "smtp",
      "action_type": "send_email",
      "config": {
        "to": "oncall@example.com",
        "subject": "CRITICAL: {{transformed.summary}}",
        "body": "Downstream step status: {{steps.slack_alert.status}}"
      }
    }
  ],
  "rate_limit": 60,
  "max_concurrency": 5,
  "concurrency_key": "payload.service_id",
  "priority": 10
}
```

---

## 2. Trigger Types (`trigger`)

| Trigger Type | Description | Key Configuration Parameters |
| :--- | :--- | :--- |
| `webhook` | Ingests inbound HTTP POST webhooks | `secret`, `signature_header`, `signature_algorithm` (`hmac-sha256`, `slack-v0`, `stripe-v1`) |
| `http_poll` | Periodic outbound JSON polling with ETag / Last-Modified caching | `url`, `interval` (`30s`, `5m`), `items_path`, `id_field`, `headers` |
| `schedule` | Background cron or interval tick generator | `interval` (e.g. `5m`, `1h`) or standard 5-field `cron` (e.g. `*/15 * * * *`) |
| `telegram` | Ingests Telegram bot updates, channel posts, and mentions | `source_id`, `event_types` (`telegram.message`, `telegram.channel_post`) |
| `github` | Ingests GitHub webhooks | `event_types` (`issues.opened`, `push`, `pull_request.closed`) |
| `rss` | Polls RSS/Atom XML feeds for new published items | `feed_url`, `poll_interval` |

---

## 3. Rich Condition Operators (`conditions`)

Conditions evaluate Boolean expressions against incoming events without side effects:

### Supported Operators

| Operator | Description | Example Condition |
| :--- | :--- | :--- |
| `equals` / `eq` | Strict equality | `{"field": "payload.status", "operator": "equals", "value": "active"}` |
| `not_equals` / `neq` | Inequality | `{"field": "payload.env", "operator": "not_equals", "value": "dev"}` |
| `contains` | Substring or array element containment | `{"field": "payload.message", "operator": "contains", "value": "error"}` |
| `starts_with` | Prefix matching | `{"field": "payload.branch", "operator": "starts_with", "value": "release/"}` |
| `ends_with` | Suffix matching | `{"field": "payload.filename", "operator": "ends_with", "value": ".tar.gz"}` |
| `exists` | Field is present and non-null | `{"field": "payload.customer_id", "operator": "exists"}` |
| `not_exists` | Field is absent or null | `{"field": "payload.deleted_at", "operator": "not_exists"}` |
| `is_empty` | String, array, or object has length 0 | `{"field": "payload.items", "operator": "is_empty"}` |
| `is_not_empty` | String, array, or object has length > 0 | `{"field": "payload.items", "operator": "is_not_empty"}` |
| `greater_than` / `gt` | Numeric comparison (>) | `{"field": "payload.amount", "operator": "greater_than", "value": 100}` |
| `greater_or_equal` / `gte` | Numeric comparison (>=) | `{"field": "payload.retry_count", "operator": "greater_or_equal", "value": 3}` |
| `less_than` / `lt` | Numeric comparison (<) | `{"field": "payload.latency_ms", "operator": "less_than", "value": 200}` |
| `less_or_equal` / `lte` | Numeric comparison (<=) | `{"field": "payload.risk_score", "operator": "less_or_equal", "value": 0.5}` |
| `between` | Numeric range check `[min, max]` inclusive | `{"field": "payload.code", "operator": "between", "value": [200, 299]}` |
| `in` | Value exists in array | `{"field": "payload.status", "operator": "in", "value": ["queued", "running"]}` |
| `not_in` | Value is absent from array | `{"field": "payload.status", "operator": "not_in", "value": ["cancelled", "failed"]}` |
| `regex` | RE2 regular expression match | `{"field": "payload.email", "operator": "regex", "value": "^[a-z0-9._%+-]+@company\\.com$"}` |
| `time_window` | Time window check in IANA timezone | `{"operator": "time_window", "value": {"start": "09:00", "end": "17:00", "timezone": "America/New_York"}}` |

### JSONPath Array Indexing & Quantifiers

Access array elements directly using bracket notation or wildcards:
- Specific index: `payload.users[0].email`
- Wildcard ANY quantifier: `payload.items[*].price` with operator `greater_than`
- Logical composition: Nest conditions using `all` (AND) or `any` (OR):

```json
{
  "all": [
    {"field": "payload.severity", "operator": "equals", "value": "critical"},
    {
      "any": [
        {"field": "payload.tier", "operator": "equals", "value": "enterprise"},
        {"field": "payload.vip", "operator": "equals", "value": true}
      ]
    }
  ]
}
```

---

## 4. Transformations & Safe Template Filters

Transformations format the event payload before action execution.

### Transformation Modes (`type`)

- `mapping`: Map output fields to template strings or paths via `field_maps`.
- `pick`: Extract a subset of keys (`pick_keys: ["id", "name", "email"]`).
- `omit`: Remove sensitive or unwanted keys (`omit_keys: ["password", "token"]`).
- `flatten`: Flatten nested JSON objects to dot-notation keys.
- `merge`: Deep merge static configuration dictionary with the event payload (`data: {...}`).
- `compute`: Perform pure arithmetic or string concatenation (`target`, `op`, `args`).
- `passthrough`: Forward raw payload without modification.

### Template Filter Pipeline

Expressions support UNIX-style pipes with arguments: `{{ <path> | <filter>:<arg> }}`.

| Filter | Description | Example | Output |
| :--- | :--- | :--- | :--- |
| `upper` | Convert to UPPERCASE | `{{payload.name \| upper}}` | `"KEROB"` |
| `lower` | Convert to lowercase | `{{payload.name \| lower}}` | `"kerob"` |
| `trim` | Trim whitespace | `{{payload.msg \| trim}}` | Trimmed text |
| `truncate:N` | Truncate to N characters with ellipsis | `{{payload.title \| truncate:10}}` | `"Deploying..."` |
| `default:val` | Fallback if nil or empty string | `{{payload.env \| default:"production"}}` | `"production"` |
| `replace:a:b` | Replace substring `a` with `b` | `{{payload.text \| replace:"old":"new"}}` | Replaced text |
| `split:sep` | Split string into array | `{{payload.tags \| split:","}}` | `["go", "svelte"]` |
| `join:sep` | Join array into string | `{{payload.tags \| join:" & "}}` | `"go & svelte"` |
| `json_encode` | Serialize object or array to JSON string | `{{payload.meta \| json_encode}}` | JSON string |
| `url_encode` | Percent-encode URL query string | `{{payload.query \| url_encode}}` | `"hello%20world"` |
| `base64` | Base64-encode string payload | `{{payload.token \| base64}}` | Base64 string |
| `date_format:fmt` | Format RFC3339 timestamp using Go format layout | `{{payload.time \| date_format:"2006-01-02"}}` | `"2026-10-30"` |
| `number_format:dec` | Format floating-point number to N decimals | `{{payload.price \| number_format:2}}` | `"19.99"` |
| `regex_extract:re` | Extract first captured regex group | `{{payload.log \| regex_extract:"ID:([0-9]+)"}}` | `"12345"` |
| `coalesce` | First non-empty value | `{{payload.alt \| coalesce:payload.primary}}` | First valid string |
| `length` | Return string or array length | `{{payload.items \| length}}` | `3` |

---

## 5. Multi-Step Execution Pipeline & Control Flow

Actions execute sequentially with inter-step data flow, isolated error handling policies, and step resumption.

### Inter-Step References

Steps can reference earlier step results:
- `{{steps.<id>.output.<field>}}`: Output returned by step `<id>`.
- `{{steps.<id>.status}}`: Execution status (`success`, `failed`, `skipped`).
- `{{steps.<id>.error}}`: Error message if step failed.
- `{{secrets.<name>}}`: Decrypted secret from credential vault (never logged).

### Step Error Policies (`on_error`)

- `fail` (default): Step failure immediately stops the workflow and records a failure/retry.
- `continue`: Logs warning, sets status to `failed`, and continues execution with subsequent steps.
- `skip_rest`: Gracefully stops the pipeline without failing the workflow.

### Control Flow Primitives

#### 1. Branching Step (`branch`)
Conditionally executes different step branches:
```json
{
  "id": "route_by_tier",
  "type": "branch",
  "cases": [
    {
      "when": {"field": "payload.vip", "operator": "equals", "value": true},
      "steps": [
        {"id": "notify_exec", "integration": "slack", "action_type": "post_message", "config": {"channel": "#vip-alerts"}}
      ]
    }
  ],
  "default": [
    {"id": "notify_standard", "integration": "slack", "action_type": "post_message", "config": {"channel": "#general"}}
  ]
}
```

#### 2. Loop Iteration (`for_each`)
Iterates over an array in the payload with a safety cap (up to 100 iterations):
```json
{
  "id": "loop_recipients",
  "type": "for_each",
  "items": "payload.subscribers",
  "max_iterations": 50,
  "steps": [
    {
      "id": "send_newsletter",
      "integration": "smtp",
      "action_type": "send_email",
      "config": {
        "to": "{{item.email}}",
        "subject": "Weekly Digest",
        "body": "Hello {{item.name}}"
      }
    }
  ]
}
```

#### 3. Asynchronous Delay (`delay` / `wait`)
Schedules workflow execution resumption via background worker queue without blocking threads:
```json
{
  "id": "cool_down",
  "type": "delay",
  "duration": "10m"
}
```

#### 4. Parallel Fan-Out (`parallel`)
Executes independent action branches concurrently with bounded concurrency:
```json
{
  "id": "parallel_fanout",
  "type": "parallel",
  "concurrency": 4,
  "steps": [
    {"id": "ping_slack", "integration": "slack", "action_type": "post_message", "config": {...}},
    {"id": "ping_teams", "integration": "teams", "action_type": "send_card", "config": {...}},
    {"id": "ping_email", "integration": "smtp", "action_type": "send_email", "config": {...}}
  ]
}
```

---

## 6. Action Integration Catalog

All outbound actions execute through the SSRF-safe dialer (`ssrf.SafeHTTPClient`) prohibiting RFC 1918, loopback, and cloud metadata (169.254.169.254) addresses.

### 1. Slack (`slack`)
- `post_message`: Post to incoming webhook or bot token (`chat.postMessage`).
- Config: `channel`, `text`, `username`, `icon_emoji`, `blocks`.

### 2. Microsoft Teams (`teams`)
- `send_card`: Send Office 365 connector card / adaptive card.
- Config: `webhook_url`, `title`, `text`, `theme_color`.

### 3. SMTP Email (`smtp`)
- `send_email`: Send authenticated transactional email.
- Config: `to`, `subject`, `body`, `from`, `smtp_host`, `smtp_port` (25, 465, 587, 2525).

### 4. HTTP Request (`http`)
- `request`: Outbound webhook with automatic HMAC request signing.
- Config: `url`, `method` (`GET`, `POST`, `PUT`, `DELETE`), `headers`, `body`.
- Outbound Signing: If `signing_secret` is configured, Kerob signs requests with `X-Kerob-Signature-256` and `X-Kerob-Timestamp`.

### 5. Call Workflow (`call_workflow`)
- `invoke`: Trigger another workflow programmatically with cycle and depth detection (max depth 5).
- Config: `target_workflow_id`, `payload`.

---

## 7. Reliability, Traffic Control & Observability

- **Rate Limiting**: `rate_limit` defines maximum workflow executions permitted per minute.
- **Max Concurrency**: `max_concurrency` bounds parallel executions of the same workflow.
- **Concurrency Key**: `concurrency_key: "payload.chat_id"` serializes executions per entity.
- **Priority Claiming**: `priority` (e.g. 10=high, 0=normal) ensures critical jobs are claimed first.
- **Circuit Breaker**: Each integration automatically trips to `open` after 5 consecutive failures, guarding upstream services.
- **Dry-Run API**: `POST /api/v1/workflows/{id}/test` runs full validation, condition matching, and transformation traces without executing outbound side effects.
- **Version History & Rollback**: `GET /api/v1/workflows/{id}/versions`, `POST /api/v1/workflows/{id}/rollback/{version}`.
