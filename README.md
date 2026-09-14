# AgentFleet

Local-first platform for running a fleet of **digital colleague agents** backed by CLI engines (Claude / Codex / Cursor Agent).  
Go backend + React admin UI + MySQL. Agents work in authorized workspaces with tool-permission gates, occupancy control, and optional team workflows.

## Features (current, ~v0.2.17)

- **Home chat (E-bot)** — Streaming Q&A over allowlisted workspaces; IP allowlist; optional read-only DB query via backend APIs.
- **Managed agents** — Create agents with engine, binary path, rules prompt, workspace binding, and policy flags (write / network / rm / browser).
- **Engine adapters** — Claude (`claude -p` stream-json + `--resume`), Codex (`codex exec --json` + thread resume), Cursor Agent (generic CLI).
- **Permission proxy** — PreToolUse hook → backend classifier → browser approve/deny; session-level AI auto-review; Bark notify optional.
- **Cross-agent invoke** — `@mention` other agents; isolated runs with quotes back into the caller timeline.
- **Team workflows** — Visual DAG (React Flow): sequence, parallel, merge, condition, human review, retry; occupancy so one agent has one primary task.
- **Ops** — Admin login (JWT), conversation history, stats, command audits, agent folders, status snapshots, context/token indicators, changelog menu.
- **External task progress** — Public `POST/GET /api/agents/tasks` for other tools to report progress.

## Stack

| Layer | Choice |
|-------|--------|
| Backend | Go, Gin |
| Frontend | React, Vite |
| DB | MySQL 8 (Docker, host port `3307`) |
| Engines | Local CLIs: `claude`, `codex`, Cursor `agent` |

## Layout

```
AgentFleet/
├── db/                 # schema + versioned migrations
├── docker-compose.yml  # MySQL
├── docs/plan/          # feature design notes
├── docs/superpowers/   # specs & implementation plans
├── scripts/            # start / restart / shutdown / Bark notify
├── server/             # Go API + avatar-hook
└── web/                # React admin + home chat
```

## Quick start

Prerequisites: Docker, Go 1.22+, Node.js 20+, and at least one engine CLI on `PATH`.

```bash
# MySQL + API (:8080) + Vite (:5173)
./scripts/start.sh

# Skip Docker if MySQL is already up
./scripts/start.sh --skip-db
```

Open `http://localhost:5173`. Admin console requires login (see env below).

Other helpers: `./scripts/restart.sh`, `./scripts/shutdown.sh`.

## Configuration

Set via environment (defaults are also applied in `scripts/start.sh`). **Override all secrets in production.**

| Variable | Purpose | Typical default |
|----------|---------|-----------------|
| `AVATAR_ADDR` | HTTP listen address | `:8080` |
| `AVATAR_DB_DSN` | App MySQL DSN | `root:root@tcp(127.0.0.1:3307)/colleague_avatar?...` |
| `AVATAR_DATA_DB_DSN` | Optional read-only data DB (empty = disabled) | _(empty)_ |
| `AVATAR_ALLOWED_IPS` | Client IP allowlist (comma-separated; empty may allow all depending on auth path) | `127.0.0.1` |
| `AVATAR_WORKSPACE_ROOT` | Root for resolving allowlisted workspaces | _(set to your machine)_ |
| `AVATAR_CLAUDE_BIN` | Claude CLI path | `claude` |
| `AVATAR_TIMEOUT_SEC` | Per-ask timeout (seconds) | `28800` |
| `AVATAR_PERMISSION_ENABLED` | Tool permission proxy (`0` = off) | `1` |
| `AVATAR_PERMISSION_WAIT_SEC` | Seconds to wait for human decide | `120` |
| `AVATAR_REVIEW_TIMEOUT_SEC` | Auto-reviewer timeout | `20` |
| `AVATAR_HOOK_BIN` | PreToolUse hook binary | `server/bin/avatar-hook` (built by start script) |
| `AVATAR_BACKEND_URL` | URL hook uses to reach the API | `http://127.0.0.1:8080` |
| `AVATAR_ADMIN_USER` / `AVATAR_ADMIN_PASS` | Admin console credentials | _(change me)_ |
| `AVATAR_JWT_SECRET` | HS256 secret for admin JWT | _(change me)_ |
| `AVATAR_JWT_TTL_HOURS` | Admin token TTL | `168` |
| `AVATAR_BARK_NOTIFY` | Push pending-permission alerts | `1` |
| `AVATAR_NOTIFY_SCRIPT` | Bark helper script | `scripts/notify-permission.sh` |
| `BARK_KEY` | Bark device key (if notify enabled) | _(optional)_ |

Database name / Docker volume still use the historical `colleague_avatar` identifiers; the product name is **AgentFleet**.

## Permission flow

```
engine CLI ──PreToolUse hook──▶ avatar-hook ──POST /api/permissions/request──▶ classifier
                                                                              │ ask
   hook stdout ◀── decide ◀── Hub.Wait ◀── /api/permissions/decide ◀── SSE ── browser
                                 │
                                 └─(auto on)→ Reviewer → ALLOW or fall back to modal
```

`--permission-prompt-tool` is not used: it needs a connected MCP tool; a bare subprocess would silently deny. PreToolUse hooks are the controllable entry point.

## API surface (high level)

**Public / LAN**

- `POST /api/question` — SSE home chat
- `POST /api/conversations`, `GET /api/workspaces`
- `POST|GET /api/agents/tasks`, `GET /api/agents/task-types`
- `POST /api/auth/login`

**Admin (JWT)**

- Conversations, stats, commands
- `/api/admin/agents*` — CRUD, ask/stop/new/compress, folders, status/context, audits
- `/api/admin/workflows*` — CRUD, runs, retry/resume/review, permission pending
- Test-server helpers (restricted by IP)

`GET /health` — liveness

## Security notes

- Prefer least privilege: bind workspaces, disable write/network/rm/browser unless needed.
- Treat user messages as untrusted data (prompt-injection resistant prompts in home agent).
- Configure `AVATAR_ALLOWED_IPS` for any non-localhost exposure.
- Never commit real `.env` secrets; rotate default JWT / admin passwords before sharing a host.

## License

See [LICENSE](./LICENSE) in this repository.
