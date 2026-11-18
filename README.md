# Supa-Agent

A Go modular monolith for authenticated agent management and streaming chat. It
uses Eino and DeepSeek for execution, MySQL for accounts, agents, and long-term memory,
Redis Streams for temporary run events, and local JSONL files for conversations.

## Architecture

Requests flow from `api` to application `service` objects. Services own use cases
and authorization and depend on small interfaces. `repo` implements MySQL,
Redis, and filesystem persistence. `agent` validates configurations, constructs
Eino agents, and consumes their output without depending on storage or HTTP.
`model` contains shared records and errors; `security` owns passwords and JWTs.
`llm` resolves models and wraps providers. `config` reads command settings, and
`infra` opens shared infrastructure connections. Commands assemble dependencies
explicitly and own their shutdown.

The existing `chat` command remains a standalone model CLI. The `server` command
adds the API and background run coordinator. Only `chat_model` agents are
executable; the other persisted type names remain reserved.

## Setup

Install Go 1.26.4 or later, MySQL 8+, and Redis 6.2+. Create a MySQL database and
an application account with access to it. Apply the existing migrations in order
using your MySQL client:

1. `internal/sql/000001_create_users.up.sql`
2. `internal/sql/000002_create_agents.up.sql`
3. `internal/sql/000003_create_agent_memories.up.sql`

Startup does not apply migrations. On an existing Supa database, apply migration
`000003` before enabling long-term memory or calling the memories API.

Copy `.env.example` to `.env`, fill in the MySQL connection and DeepSeek key, and
set `JWT_SECRET` to a cryptographically random value. For example, generate a
32-byte random secret in PowerShell:

```powershell
[Convert]::ToBase64String([System.Security.Cryptography.RandomNumberGenerator]::GetBytes(32))
```

The DSN must include `parseTime=true`. Process environment variables take
precedence over `.env`. Model IDs and endpoints remain in `configs/llm.yaml`;
credentials are injected separately and are never stored in that catalog.

| Setting | Default |
| --- | --- |
| `MYSQL_DSN` | Required |
| `DEEPSEEK_API_KEY` | Required |
| `JWT_SECRET` | Required, at least 32 random bytes |
| `HTTP_ADDR` | `127.0.0.1:8080` |
| `JWT_ACCESS_TTL` | `15m` |
| `REDIS_ADDR` | `127.0.0.1:6379` |
| `REDIS_USERNAME`, `REDIS_PASSWORD` | Empty |
| `REDIS_DB` | `0` |
| `SESSION_DIR` | `data/sessions` |
| `LLM_CONFIG` | `configs/llm.yaml` |
| `RUN_TIMEOUT` | `5m` |
| `RUN_MAX_CONCURRENT` | `8` |
| `SHUTDOWN_TIMEOUT` | `10s` |

Run these commands from the repository root:

```powershell
go run ./cmd/server
go run ./cmd/chat -list-models
go run ./cmd/chat -stream "Explain what an agent does."
```

Model listing does not load `.env` or require any credentials or infrastructure.

## HTTP API

JSON endpoints use `Content-Type: application/json`. Authenticated endpoints
require `Authorization: Bearer <access_token>`. Input rejects unknown top-level
fields and trailing JSON. Authentication bodies are limited to 8 KiB; agent and
run bodies are limited to 64 KiB. Errors use `{"error":"message"}`.

| Method and route | Behavior |
| --- | --- |
| `GET /healthz` | Process health |
| `POST /users` | Register with `{email,password}`; returns 201 and signs in |
| `POST /auth/tokens` | Log in with `{email,password}` |
| `GET /users/me` | Current account; also supports HEAD |
| `POST /agents` | Create `{type,config}`; returns 201 and `Location` |
| `GET /agents?limit=20&offset=0` | Owner's agents, ascending ID; maximum limit 100 |
| `GET /agents/{id}` | Owner's definition |
| `PUT /agents/{id}` | Replace `{type,config}` |
| `DELETE /agents/{id}` | Delete an owned definition; returns 204 |
| `POST /agents/{id}/runs` | Start `{content,session_id?}`; returns 202 and `Location` |
| `GET /agents/{id}/memories?limit=20&offset=0` | Active and archived memories for an owned agent, most recently updated first |
| `POST /agents/{id}/memories/{memory_id}/archive` | Exclude a memory from recall; returns 204 |
| `DELETE /agents/{id}/memories/{memory_id}` | Permanently remove a memory; returns 204 |
| `GET /runs/{id}` | Run descriptor |
| `GET /runs/{id}/events` | Replay and follow authenticated SSE |
| `POST /runs/{id}/cancel` | Cancel; 202 while running, 200 when terminal; safe to repeat |
| `GET /sessions` | Owner's session metadata, newest first |
| `GET /sessions/{id}` | Session metadata and chronological messages |
| `DELETE /sessions/{id}` | Delete an inactive conversation; returns 204 |

Registration and login return `{user,access_token,token_type,expires_in}`.
Passwords and password hashes are never returned. Agent definitions retain
`{id,user_id,type,config}`. An omitted model stays omitted in persisted config
and uses the gateway default at execution time; provider failures do not switch
models automatically.

Missing and other-owner resources return the same 404. A second run or deletion
on an active session returns 409. Runs in different sessions can execute
concurrently, up to the configured limit; excess submissions return 429.

## NexusAgent memory and basic tools

Enable them on a `chat_model` agent when creating it or replacing its config.

```json
{
  "type": "chat_model",
  "config": {
    "name": "assistant",
    "instruction": "Be helpful and concise.",
    "tools": ["calculator", "datetime"],
    "memory_window": 20,
    "memory_long_enabled": true
  }
}
```

- `tools` enables only the named Eino tools. `calculator` evaluates arithmetic.
  `datetime` supports `now`, `add_days`, and `diff`, which counts calendar days
  until January 1 of the next year. Its default time zone is `Asia/Shanghai`.
- `memory_window` enables short-term compaction at 1–200 messages. Eino
  summarizes older turns when the window or estimated 8,192-token budget is
  exceeded. Recent turns keep their associated tool calls and results. The
  full transcript stays available through the sessions API.
- `memory_long_enabled` learns durable user facts after a successful turn has
  been saved and recalls relevant facts in later sessions of the same agent.
  With no explicit window, it also enables compaction with a 20-message window.
  The agent's configured model selects up to three facts from the 40 most
  recently updated active memories in MySQL. Supa uses model-based recall in
  place of NexusAgent's embedding and Milvus search.

Omitted settings keep the existing chat behavior. Memory model calls are
bounded and best effort. Disabling long-term memory stops extraction and recall.
Use the authenticated memories API to review, archive, or delete saved facts.
Deleting an agent removes its saved facts. Legacy NexusAgent memory rows require
an explicit ownership mapping before import.

## Streaming example

This PowerShell example registers a user, creates an agent, starts a conversation,
and subscribes using `curl.exe`:

```powershell
$base = 'http://127.0.0.1:8080'
$account = Invoke-RestMethod -Method Post -Uri "$base/users" -ContentType 'application/json' -Body '{"email":"you@example.com","password":"your-password"}'
$headers = @{ Authorization = "Bearer $($account.access_token)" }
$agent = Invoke-RestMethod -Method Post -Uri "$base/agents" -Headers $headers -ContentType 'application/json' -Body '{"type":"chat_model","config":{"name":"assistant","instruction":"Be helpful and concise."}}'
$run = Invoke-RestMethod -Method Post -Uri "$base/agents/$($agent.id)/runs" -Headers $headers -ContentType 'application/json' -Body '{"content":"Explain AI agents."}'
curl.exe -N -H "Authorization: Bearer $($account.access_token)" "$base$($run.events_url)"
```

The accepted run contains `{run_id,agent_id,session_id,status,events_url}`. For a
follow-up, submit a new run with its `session_id` and new `content`. The server
loads the conversation; clients do not submit system messages or full histories.

SSE frames contain `id`, `event`, and a JSON `data` line. Event names are
`run_started`, `message_delta`, `message`, `message_end`, `error`, and `done`.
`message_delta` contains Eino message chunks, grouped by sequential `message_id`;
`message_end` ends that message, while `done` ends the entire run. A terminal
`error` contains a sanitized reason. Subscribe without a cursor to replay all
events. Reconnect with the last received Redis entry ID:

```powershell
curl.exe -N -H "Authorization: Bearer $($account.access_token)" -H 'Last-Event-ID: 1730000000000-0' "$base$($run.events_url)"
```

Use an ID from the actual stream. Heartbeat comments arrive after 15 seconds of
inactivity. Subscription disconnects do not cancel generation. Browser clients
can use streaming `fetch` with the authorization header and retain event IDs for
reconnects. Subscribe again after refreshing an expired access token.

## Persistence and lifecycle

One API process owns a session directory. Conversations live at
`SESSION_DIR/<user_id>/<session_id>.jsonl`. The first record is the existing
session header, followed by complete Eino messages, one per line. Snapshots are
copied before execution, and appends are synced before updating cached history.
Existing flat JSONL files are left untouched; importing them requires an explicit
ownership mapping and placement into the corresponding user directory while the
server is stopped.

The user message is saved before acceptance. Completed output is saved before
`done`; a failed or canceled generation keeps the user message and does not save
partial output. Filesystem, Redis, and model operations do not form a distributed
transaction: a lost response can leave an accepted run active, and a failed
terminal-event write can follow a committed complete response. Read the run and
session state before retrying an uncertain submission.

Redis stores metadata and events under `agent:runs:{run_id}:meta` and
`agent:runs:{run_id}:events`. Every append refreshes both keys to one hour; events
are not trimmed. Terminal status and its event are recorded atomically, and no
events can follow termination. After expiration, run/status subscriptions return
404; JSONL conversations remain until deletion.

Run states are `running`, `succeeded`, `failed`, and `canceled`. Runs execute in
background goroutines under an application-owned deadline. They are not durable
jobs: startup marks retained running records as failed instead of resuming them.
Do not share the Redis run namespace or session directory between API instances.
Shutdown stops new submissions, cancels runs, lets SSE handlers finish, and closes
infrastructure after bounded worker/HTTP shutdown.

## Validation

```powershell
go build ./...
go vet ./...
go run ./cmd/chat -list-models
```
