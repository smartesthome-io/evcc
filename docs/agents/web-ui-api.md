# Web UI & REST API

## Server Architecture

- **Router:** gorilla/mux, strict slash
- **Middleware:** GZIP, CORS (`*`), ETag caching, request logging, JSON headers, JWT auth
- **Timeouts:** Read 5s, Write 10s, Idle 120s
- **Static assets:** embedded in binary (`fs.FS`)
- **Default port:** 7070

## Frontends

- First-party UI lives in `ui/classic/` (sources, tests, `dist/` embedded by `main.go`). Third-party UIs live in `custom-ui/<name>/` and are loaded at runtime from `ui.customDir`, never embedded. They use only the HTTP API and WebSocket.
- Every installed UI is served at `/ui/<name>/` (`/ui/classic/` always exists). Relative `api`, `ws`, `i18n`, `providerauth`, `globals.js`, `custom.css`, `custom-logo-*` below a mount are aliased to the backend (`server/ui.go`).
- `/` serves the UI from the `evcc-ui` cookie, else `ui.default` (classic stays at `/`, others redirect to their mount). The cookie only records a deviation from the default and is set by visiting a mount.
- A custom UI needs a `ui.json` (`name` equal to the directory, optional `title`, `entry`). Names match `^[a-z0-9-]+$`; `classic`, `api`, `ws`, `assets`, `meta`, `i18n` are reserved.
- OAuth callbacks redirect to `/ui/classic/#/config`.
- Shared Playwright harness: `testkit/`. Specs per UI: `ui/<name>/tests`.

## REST API (base `/api/`)

### Site-level
- `POST /buffersoc/{value}`, `/prioritysoc/{value}`, `/residualpower/{value}` etc.
- `GET /tariff/{tariff}` — tariff rates
- `GET /sessions` — charging history
- `GET /state` — complete system state (supports jq filtering)

### Per-loadpoint (`/loadpoints/{id}/...`)
- `POST mode/{value}` — off/smart/now (deprecated aliases pv/minpv)
- `POST alwayscharge/{value}` — off/on/once
- `POST limitsoc/{value}`, `limitenergy/{value}` — charge limits
- `POST mincurrent/{value}`, `maxcurrent/{value}` — current limits
- `POST phases/{value}` — phase config
- `POST priority/{value}`, `batteryboost/{value}`
- `POST plan/energy/{value}/{time}` — schedule plan
- `POST vehicle/{name}` — select vehicle
- `POST smartcostlimit/{value}` — smart cost threshold
- `POST solarshare/{value}` — required solar share of min power (default 1)

### Configuration (`/config/...`, auth required)
- CRUD for devices (chargers, meters, vehicles, tariffs)
- Template browsing and testing
- Site, loadpoint, circuit, HEMS, messaging config
- `GET /config/evcc.yaml` — YAML export

### System (`/system/...`, auth required)
- Log viewing (`/log`, `/log/areas`), cache clear, shutdown

### Database (`/db/...`, auth + second factor required)
- Backup download, restore from file, selective reset
- Second factor: admin password in request body, or API key via Bearer token (bypasses password check)

### Handler Pattern
Generic `handler[T]` with type conversion, setter, getter.
Specialized: `floatHandler`, `intHandler`, `boolHandler`, `durationHandler`.

## WebSocket (`/ws`)

- `coder/websocket` (RFC 6455)
- Pub/sub via `SocketHub`
- Buffered channels (1024 per subscriber)
- Welcome message with full state snapshot
- Incremental updates as JSON key-value pairs with dot-notation keys:
  ```json
  {"loadpoints.1.mode": "solar", "site.gridPower": 1234}
  ```
- Write timeout: 10s, compression (disabled for Safari)

## State Flow

1. WS connects -> receives welcome with full state
2. App emits `util.Param` on changes
3. Hub broadcasts to subscribers
4. Frontend `store.update(msg)` merges via dot-notation
5. Components reactively re-render

## Authentication

- JWT, 90-day lifetime
- HttpOnly cookie (`auth`) with `SameSite=Strict`
- Also accepts `Authorization: Bearer <token>` header
- Modes: Disabled, Locked (demo), Configured (password)
- Protects `/api/config`, `/api/system`, and `/api/db`

## MQTT Integration

- Publishes state changes to configurable broker
- Subscribes to control topics
- Retained messages for state persistence

## Key Files

- `server/http.go` — router setup
- `server/http_auth.go` — authentication
- `server/http_site_handler.go` — state + request handlers
- `server/http_config_*.go` — config endpoints
- `server/http_loadpoint_handler.go` — per-loadpoint endpoints
- `server/socket.go` — WebSocket pub/sub
- `assets/js/app.ts` — Vue app entry
- `assets/js/store.ts` — reactive state store
- `assets/js/api.ts` — Axios clients
- `assets/js/router.ts` — route definitions
