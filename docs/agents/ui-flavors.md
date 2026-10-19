# UI Flavors

The built-in UI is classic (`assets/`, embedded into the binary). Alternative UIs live in `ui/<name>/` and replace it at runtime via `--ui-dir <dir>` or `EVCC_UI_DIR`. The directory must contain `index.html`.

## Contract

A flavor is a static bundle served by the backend. It must honor:

- `index.html` is templated with `[[.Version]]` and `[[.DefaultLang]]` (`server/http_site_handler.go`)
- Static files live under `/assets` and `/meta`. These prefixes are hardcoded in `server/http.go`
- `/globals.js` provides `window.evcc` (server-injected config), load it before the app
- REST under `/api` (`server/openapi.yaml`), state stream on `/ws` (types in `assets/js/types/evcc.ts`)
- Translations from `/i18n/<lang>.json`, auth flows via `/providerauth`, optional `/custom.css`

## Build and ship

- `make ui-flavor FLAVOR=<name>` builds `ui/<name>/dist`
- `make container-flavor FLAVOR=<name>` builds `evcc:<name>` with podman (override with `CONTAINER=docker`) from `Dockerfile.flavor` on top of a base evcc image
- Run locally: `evcc --ui-dir ui/<name>/dist`

## Rules

- Do not modify `assets/` for flavors, it is upstream-owned
- Extract shared code (API client, types) only once a second flavor needs it
