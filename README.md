# Damtie: Multi-Channel Telegram Mirroring Engine

> **Damtie is an event-driven Telegram mirroring engine.** One Telegram bot can monitor multiple Telegram channels and groups, route incoming posts according to configurable rules, and automatically deliver them to one or more destination channels or groups.

```text
                     Telegram Bot
                          │
          ┌───────────────┼───────────────┐
          ↓               ↓               ↓
      Source A        Source B        Source C
     (Monitored)     (Monitored)     (Monitored)
          │               │               │
          └───────────────┼───────────────┘
                          ↓
                    Damtie Engine
                     (Idempotency)
                          │
                    Routing Rules
                          │
          ┌───────────────┼───────────────┐
          ↓               ↓               ↓
      Target X        Target Y        Target Z
    (Backup / Sync) (Archive / Rep) (Public Mirror)
```

Damtie is **not** a basic script or a naive polling loop. It is a robust, decoupled, event-driven engine providing persistent state, strict idempotency, album/media-group buffering, exponential backoff retries, dead-letter tracking, Telegram rate limiting, and an operator management API with a live web dashboard.

---

## 1. Core Principles

- **One Bot / Many Channels**: A single bot application connects to Telegram Bot API, monitors N sources, and replicates to N destinations according to dynamic database routes.
- **Pure Verbatim Mirroring**: Damtie preserves original content as closely as the Telegram API allows. It does **not** introduce AI summarization, text rewriting, opportunity extraction, or automated footers.
- **Copy vs. Forward**: By default, Damtie uses Telegram's native `copyMessage` and `copyMessages` APIs. This duplicates the message, media, captions, entities, and formatting **without** the "Forwarded from..." banner, so mirrored posts look native.
- **Strict Deduplication & Idempotency**:
  - **Source level**: Uniquely keyed by `telegram:<chat_id>:<message_id>` (or `telegram:<chat_id>:mg:<media_group_id>` for albums). Duplicate updates are ignored.
  - **Destination level**: Unique constraint on `(event_id, destination_id)`. The destination message ID is recorded; if a failed job is retried after delivery, the recorded result prevents duplicate posts.
- **Album / Media Group Preservation**: Posts sharing a `media_group_id` are buffered within a sliding window and published as a unified Telegram album rather than isolated single posts.
- **Decoupled Architecture**: Telegram is an adapter layer. The underlying Damtie engine (ingestion, event model, routing, PostgreSQL persistence, worker pool) remains completely reusable.

---

## 2. System Architecture

```text
               Telegram Long-Poll Updates
                          │
                          ▼
             ┌─────────────────────────┐
             │ Damtie Telegram Listener│ (buffers albums & filters monitored chats)
             └────────────┬────────────┘
                          │ RawPost JSON
                          ▼
             ┌─────────────────────────┐
             │ Telegram Source Adapter │ (normalizes to generic Event)
             └────────────┬────────────┘
                          │
                          ▼
             ┌─────────────────────────┐
             │   Ingestion Pipeline    │ (persists Event with unique external ID)
             └────────────┬────────────┘
                          │
                          ▼
             ┌─────────────────────────┐
             │     Routing Engine      │ (resolves matching active routes)
             └────────────┬────────────┘
                          │
                          ▼
             ┌─────────────────────────┐
             │ Delivery Store (Jobs)   │ (enqueues pending jobs in PostgreSQL)
             └────────────┬────────────┘
                          │
       FOR UPDATE SKIP LOCKED (concurrent workers)
                          │
                          ▼
             ┌─────────────────────────┐
             │ Background Worker Pool  │
             └────────────┬────────────┘
                          │
                          ▼
             ┌─────────────────────────┐
             │Telegram Destination Adp.│ (rate-limited, copies post/album)
             └────────────┬────────────┘
                          │
                          ▼
             Destination Telegram Channels
```

```mermaid
flowchart TD
    subgraph TelegramSources["Telegram Channels & Groups"]
        ChA["Source Channel A"]
        ChB["Source Group B"]
        ChC["Source Channel C"]
    end

    subgraph DamtieIntegration["Telegram Integration Layer"]
        Listener["Telegram Listener\n(Long-poll getUpdates)"]
        Buffer["Media Group Buffer\n(Album Aggregator)"]
        SrcAdp["Telegram Source Adapter\n(Normalize to Event)"]
    end

    subgraph CoreEngine["Damtie Core Engine"]
        Pipe["Ingestion Pipeline"]
        Router["Routing Engine"]
        Workers["Delivery Worker Pool\n(Exponential Backoff)"]
    end

    subgraph Storage["PostgreSQL Database"]
        Events[("events\n(uq_events_source_external)")]
        Routes[("routes\n(bindings & filters)")]
        Jobs[("delivery_jobs\n(uq_event_destination)")]
        State[("telegram_update_state\n(offset tracking)")]
    end

    subgraph Targets["Mirrored Targets"]
        Dest1["Backup Channel A"]
        Dest2["Backup Group B"]
        Dest3["Archive Channel C"]
    end

    ChA & ChB & ChC --> Listener
    Listener --> Buffer --> SrcAdp --> Pipe
    Pipe --> Events
    Pipe --> Router
    Routes -.-> Router
    Router --> Jobs
    Workers -. Claim with SKIP LOCKED .-> Jobs
    Workers --> DestAdp["Telegram Destination Adapter\n(Rate-Limited copyMessage/s)"]
    DestAdp --> Targets
    DestAdp -. Result (Message IDs) .-> Workers
    Workers -. Update result & status .-> Jobs
    Listener -. Confirm offset .-> State
```

---

## 3. Telegram Bot Setup & Permissions Guide

### Step 1: Create a Bot via @BotFather
1. Open Telegram and message [@BotFather](https://t.me/BotFather).
2. Send `/newbot` and follow instructions to choose a name and username.
3. Save the **HTTP API Bot Token** (e.g. `123456789:ABCdefGHIjklMNOpqrsTUVwxyz`).
4. To allow the bot to read group messages without being explicitly mentioned:
   - Send `/setprivacy` to @BotFather.
   - Select your bot.
   - Choose **Disable** (or ensure the bot is added as an Administrator in your groups).

### Step 2: Understanding Telegram Bot Permissions

| Context | Required Role | Required Permissions | Notes |
| :--- | :--- | :--- | :--- |
| **Source Channel** | Administrator | Read Messages / Channel Posts | Telegram Bot API **does not deliver channel posts** to bots unless the bot is an Administrator in the channel. |
| **Source Group / Supergroup** | Member or Admin | Read Messages | If Privacy Mode is enabled, bot must be Admin to see all posts. If Privacy Mode is disabled, bot can be normal member. |
| **Destination Channel** | Administrator | **Post Messages**, Edit Messages | Bot must have rights to publish new posts. |
| **Destination Group** | Member or Admin | Send Messages, Send Media | Ensure group permissions allow bots to send media and links. |

### Step 3: Determining Telegram Chat IDs
Telegram channel and supergroup IDs are 64-bit integers usually prefixed with `-100` (e.g., `-1001234567890`).
To find a chat ID:
- Forward a message from the channel to [@userinfobot](https://t.me/userinfobot) or [@JsonDumpBot](https://t.me/JsonDumpBot).
- Or add your bot to the chat and make a request to `GET http://localhost:8080/sources/{id}/access` after registering.

---

## 4. Quickstart with Docker Compose

### 1. Clone & Configure
```bash
git clone https://github.com/Samuel-0228/Damtie.git
cd Damtie
cp .env.example .env
```

Edit `.env` and fill in your Bot Token:
```env
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrsTUVwxyz
DATABASE_URL=postgres://postgres:postgres@postgres:5432/automation?sslmode=disable
```

### 2. Launch Services
```bash
docker compose up -d
```
This spins up:
- PostgreSQL 16 (with automatic migrations applied on startup)
- Damtie Engine & Telegram Listener (`http://localhost:8080`)

### 3. Open the Management Dashboard
Navigate to `http://localhost:8080/` in your browser to view the technical dashboard showing sources, destinations, active routes, and real-time delivery statistics.

---

## 5. Operator Management API

Damtie provides a clean REST API for dynamic source, destination, and route configuration without restarting the service.

### Sources (`/sources`)

#### Add a Monitored Telegram Source
```bash
curl -X POST http://localhost:8080/sources \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Alpha Channel",
    "telegram_chat_id": "-100111222333"
  }'
```

#### Check Bot Access to Source
```bash
curl http://localhost:8080/sources/<SOURCE_ID>/access
```
Response:
```json
{
  "accessible": true,
  "title": "Alpha Channel",
  "type": "channel",
  "status": "administrator"
}
```

#### List Sources
```bash
curl http://localhost:8080/sources
```

#### Enable / Disable Source
```bash
curl -X PATCH http://localhost:8080/sources/<SOURCE_ID> \
  -H "Content-Type: application/json" \
  -d '{"enabled": false}'
```

---

### Destinations (`/destinations`)

#### Register a Mirror Destination
```bash
curl -X POST http://localhost:8080/destinations \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Backup Archive",
    "telegram_chat_id": "-100999888777"
  }'
```

#### Check Bot Access to Destination
```bash
curl http://localhost:8080/destinations/<DESTINATION_ID>/access
```

---

### Routes (`/routes`)

Create a routing relationship between a monitored source and a destination.

#### 1 Source → 1 Destination
```bash
curl -X POST http://localhost:8080/routes \
  -H "Content-Type: application/json" \
  -d '{
    "source_id": "<SOURCE_A_ID>",
    "destination_id": "<DEST_X_ID>",
    "enabled": true
  }'
```

#### 1 Source → Multiple Destinations (Fan-out)
Simply create multiple routes with the same `source_id`:
```bash
# Route to Backup A
curl -X POST http://localhost:8080/routes \
  -H "Content-Type: application/json" \
  -d '{"source_id": "<SOURCE_A_ID>", "destination_id": "<BACKUP_A_ID>", "enabled": true}'

# Route to Public Mirror
curl -X POST http://localhost:8080/routes \
  -H "Content-Type: application/json" \
  -d '{"source_id": "<SOURCE_A_ID>", "destination_id": "<PUBLIC_MIRROR_ID>", "enabled": true}'
```
When an event arrives at Source A, it is persisted once, and separate delivery jobs are enqueued for each configured destination.

---

### Deliveries & Stats (`/deliveries`)

#### View Delivery Statistics
```bash
curl http://localhost:8080/deliveries/stats
```
Response:
```json
{
  "pending": 0,
  "processing": 0,
  "delivered": 1420,
  "failed": 2,
  "dead_letter": 0,
  "total": 1422
}
```

#### List Failed Deliveries
```bash
curl http://localhost:8080/deliveries/failed
```

#### Retry a Delivery Job
```bash
curl -X POST http://localhost:8080/deliveries/<JOB_ID>/retry
```

---

### Historical Synchronization (`POST /sources/{id}/sync`)

The default behavior is **only new posts** from the time a channel is registered. To backfill specific historical messages:
```bash
curl -X POST http://localhost:8080/sources/<SOURCE_ID>/sync \
  -H "Content-Type: application/json" \
  -d '{
    "message_ids": [1001, 1002, 1003, 1004]
  }'
```

---

## 6. Supported Content & Media Types

Damtie mirrors every Telegram message type natively supported by Bot API `copyMessage` and `copyMessages`:

- **Text & Links**: Rich markdown, HTML entities, custom emojis, links, spoilers.
- **Photos**: Full resolution photos with captions.
- **Videos & Animations**: MP4 videos, GIFs, animations with captions.
- **Documents & Files**: PDFs, archives, attachments with file names and captions.
- **Audio & Voice**: MP3/M4A music, OGG voice notes with durations.
- **Stickers & Video Notes**: Static, animated, and video stickers, round video notes.
- **Media Groups (Albums)**: Multi-photo/video albums are aggregated by `media_group_id` and replicated as a unified Telegram album.

---

## 7. Reliability, Rate Limiting & Recovery

1. **Restart Recovery**: The listener saves the confirmed Telegram `last_update_id` in PostgreSQL (`telegram_update_state`). On process restart, it resumes with `offset = last_update_id + 1`, preventing missed or duplicated posts.
2. **Delivery Idempotency**: Each delivery records the resulting Telegram destination message ID in PostgreSQL (`result`). Retrying a completed or in-doubt job recognizes the recorded result and avoids double-posting.
3. **Telegram Rate Limiter**: The built-in rate limiter enforces global limits (default 25 requests/sec) and per-chat limits (default 1 request/sec per target chat) to comply with Telegram API constraints.
4. **Flood Control (429 Retry-After)**: Telegram flood control responses (`retry_after`) are automatically intercepted, delaying retries by the requested duration.
5. **Worker Pool Concurrency**: Background workers fetch jobs using `FOR UPDATE SKIP LOCKED`, preventing contention across multiple engine replicas.

---

## 8. Configuration Reference

All settings are configured via environment variables or `.env`:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `DATABASE_URL` | `postgres://...` | PostgreSQL connection string |
| `PORT` | `8080` | REST API HTTP port |
| `TELEGRAM_BOT_TOKEN` | *required* | Telegram Bot API token from @BotFather |
| `TELEGRAM_ENABLED` | `true` | Enable or disable the Telegram listener |
| `TELEGRAM_APIBaseURL` | `""` | Optional custom Bot API endpoint (for local Bot API servers) |
| `TELEGRAM_POLL_TIMEOUT` | `30` | Long-polling timeout in seconds for `getUpdates` |
| `TELEGRAM_MEDIA_GROUP_WINDOW` | `1s` | Album buffering window before flushing media groups |
| `TELEGRAM_RATE_GLOBAL` | `25` | Maximum Telegram requests per second across all chats |
| `TELEGRAM_RATE_PER_CHAT` | `1` | Maximum Telegram requests per second targeting a single chat |
| `WORKER_POLL_INTERVAL` | `1s` | Interval for worker job polling |
| `WORKER_CONCURRENCY` | `4` | Number of concurrent delivery workers |
| `MAX_RETRIES` | `3` | Max retry attempts before moving a job to `dead_letter` |
| `INITIAL_BACKOFF` | `2s` | Initial retry backoff delay |
| `MAX_BACKOFF` | `1m` | Cap on exponential backoff delay |
| `LOG_LEVEL` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |

---

## 9. Testing

Damtie includes a comprehensive test suite covering the entire pipeline:

```bash
# Run unit and integration tests
go test -v ./...

# Run tests with the Go race detector
go test -race ./...
```

The test suite validates:
- **Multi-source routing**: Verifies Source A never leaks into routes for Source B or C.
- **Fan-out**: Verifies 1 source delivering to N destinations enqueues 1 event and N independent delivery jobs.
- **Source Deduplication**: Verifies duplicate updates produce exactly 1 event.
- **Delivery Idempotency**: Verifies retrying an already completed job does not re-post to Telegram.
- **Album Buffering**: Verifies multiple updates with matching `media_group_id` are combined and delivered via `CopyMessages`.
- **Restart Recovery**: Simulates crashes and restarts, confirming resume from offset without duplicates.
- **Rate Limiting & Backoff**: Verifies flood control compliance and exponential backoff.
# tg-channel-backup
