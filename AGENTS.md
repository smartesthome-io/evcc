# Agent Rules for evcc

evcc is an extensible EV charge controller and home energy management system: Go backend, Vue 3 + TypeScript frontend, plugin-based device integrations, local-first (no cloud dependency).

Every non-trivial change goes through the six phases below, in order. Do not skip or merge phases, and do not start a phase before the previous gate is passed.

## Development Flow

```
1 Refinement -> 2 Design -> 3 Design Review -> 4 Implementation -> 5 Testing -> 6 Implementation Review
   [gate: human]  [-]        [gate: human]       [-]                [gate: green]   [gate: human]
```

**Trivial changes** (typo, translation key, one-line fix with an obvious test, template param tweak) may collapse phases 1-3 into a two-sentence plan in the conversation. Say explicitly that you are taking the fast path. When in doubt, use the full flow.

**Gates**: a human-gated phase ends with a short summary and an explicit question. Stop and wait for approval. Silence is not approval. If a later phase uncovers a flaw in an earlier artifact, go back, fix the artifact, and re-pass its gate. Do not patch around it in code.

### 1. Refinement

Goal: agree on _what_ and _why_ before thinking about _how_.

- Read the linked issue/discussion (CONTRIBUTING requires one). Read the code that will be affected. Load the relevant `docs/agents/` files (see [Domain Knowledge](#domain-knowledge)).
- Write down: problem statement, user-visible behavior, acceptance criteria (testable, one line each), explicit non-goals, affected components, open questions.
- Ask the user about every open question that changes behavior or scope. Resolve what you can from code; do not ask what the code answers.
- Surface conflicts with existing behavior, config compatibility (`evcc.yaml`, DB, API, MQTT/WebSocket payloads), and docs/template impact.
- **Output**: a refinement note in the conversation (problem, criteria, non-goals, questions). **Gate**: user confirms scope and criteria.

### 2. Design

Goal: a concrete, reviewable plan that fits the existing architecture.

- Base the design on the standards in this file and in `docs/agents/`. Prefer extending an existing package, interface, or pattern over introducing a new one. Name the existing code you build on.
- Cover, as far as relevant: package/file placement, types and interfaces (capabilities via `api.Cap` and `implement`, generated code), data flow and concurrency (control loop, locking, context), config and template changes, DB migrations, API/OpenAPI and WebSocket changes, UI components and store usage, i18n keys, error handling, failure modes, performance on the control loop.
- List alternatives considered with a one-line reason for rejecting each.
- Include a **test plan** mapped to the acceptance criteria (which Go unit tests, which Playwright specs, which manual checks) and a **rollout/compat note** (breaking changes, migrations, deprecations).
- Keep it as small as the criteria allow. No abstractions for hypothetical future needs.
- **Output**: design document in the conversation, or `docs/design/<issue>-<slug>.md` for larger work (ask whether it should be committed). **Gate**: none, goes straight to review.

### 3. Design Review

Goal: catch problems while they are cheap.

- Review as a separate pass, ideally by a fresh-context subagent that sees only the refinement note, the design, and the code, not the author's reasoning.
- Checklist: meets every acceptance criterion; no scope creep; follows existing patterns and the standards below; concurrency and control-loop safety; failure and edge cases (NaN/Inf, missing device data, restart, partial config); backward compatibility; security (auth tier, secrets, input validation); testability; smaller alternative available?
- Findings are ranked (blocker, should-fix, nit) with a concrete suggestion each. Update the design for every blocker and should-fix, or record why not.
- **Output**: review findings plus the revised design. **Gate**: user approves the final design. Implementation does not start without it.

### 4. Implementation

Goal: build exactly the approved design.

- Work on a feature branch, never on `master`. Small, focused commits in the style described under [Conventions](#conventions).
- Follow the design. If reality forces a deviation, stop, update the design, tell the user, and get re-approval for material changes.
- Write the tests from the test plan together with the code, not afterwards.
- Never hand-edit generated files (`*_enumer.go`, `*_decorators.go`, mocks, `templates/docs`); change the source and regenerate (`make assets`, `make docs`, `make openapi`).
- No drive-by refactors, no unrelated formatting changes, no new dependencies without a reason in the design.
- **Output**: working tree with the change. **Gate**: proceed to testing.

### 5. Testing

Goal: evidence that the acceptance criteria hold, not just that the code compiles.

Run what applies and fix failures before moving on:

| Area                | Command                                                                                          |
| ------------------- | ------------------------------------------------------------------------------------------------ |
| Go unit tests       | `make test` (or `go test ./path/...` while iterating)                                            |
| Go race/vet         | `go test -race ./path/...`, `go vet ./...`                                                       |
| Go lint             | `make lint` (golangci-lint + modernize)                                                          |
| Generated artifacts | `make assets docs` then `make porcelain` (CI fails on a dirty tree)                              |
| Frontend            | `make lint-ui` (format, oxlint, vue-tsc, i18n check), `make test-ui`                             |
| Playwright          | `make ui build` first (tests use the binary), then `vpx playwright test tests/<spec>.spec.ts`    |
| Templates           | `evcc --template-type <type> --template <file>`                                                  |
| Manual UI check     | `evcc --config <file> --disable-auth` plus `vp run dev` (http://127.0.0.1:7071); exercise golden path, error states, loading states, German text length |

- Every acceptance criterion maps to at least one automated test or an explicitly stated manual check. Report which.
- Test failure paths too: errors, invalid or missing data, concurrent access, restart persistence.
- If you cannot run something (hardware, network, browser), say so. Never claim it passed.
- **Output**: a test report listing commands run and results, plus any criterion not covered. **Gate**: all checks green, no uncovered criterion without user acknowledgement.

### 6. Implementation Review

Goal: independent verification of the finished diff against the design.

- Review the full diff (`git diff master...HEAD`) in a separate pass, ideally by a fresh-context subagent (`/code-review`, `/simplify`, `/security-review` as fitting).
- Checklist: diff matches the approved design and nothing more; correctness and edge cases; error wrapping and logging (no secrets); concurrency and context handling; tests meaningful (would fail without the change); no dead code, stray debug output, or needless comments; i18n in sync; docs, schema, and OpenAPI updated; commit and PR text follow conventions.
- Fix findings, re-run affected tests from phase 5, then report: findings fixed, findings not fixed and why.
- **Output**: review summary. **Gate**: user accepts. Only then open or update the PR.

After opening a PR: watch CI until every check finishes and fix failures on the same branch. Do not argue with review bots; implement the suggestion or resolve the thread.

## Domain Knowledge

Deep documentation lives in `docs/agents/`. Load what the task needs during refinement and design:

| File                                                          | When to load                                                                 |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| [Core Domain](docs/agents/core-domain.md)                     | Control loop, loadpoint logic, PV surplus, charge modes, tariffs, interfaces |
| [Hardware Integrations](docs/agents/hardware-integrations.md) | Charger/meter/vehicle implementations, adding new devices                    |
| [Easee Architecture](docs/agents/easee-architecture.md)       | Easee charger (REST+SignalR, async correlation, concurrency)                 |
| [OCPP Forwarder](docs/agents/ocpp-forwarder.md)               | OCPP proxy/forwarder (sidecar relay, read-only mode)                         |
| [Plugin System](docs/agents/plugin-system.md)                 | Plugin layer (HTTP, MQTT, Modbus, SunSpec, JS), templates                    |
| [Web UI & API](docs/agents/web-ui-api.md)                     | REST API, WebSocket, Vue frontend, authentication                            |
| [API Security](docs/agents/api-security.md)                   | Auth modes, JWT/API key/session, two-tier checks, credential storage         |
| [Provider Auth](docs/agents/provider-auth.md)                 | Third-party logins (OAuth, device code, tokens), template `auth` block       |
| [Chart Conventions](docs/agents/charts.md)                    | ECharts charts: axes, units, tooltips, layout                                |
| [Energy Analytics](docs/agents/energy-analytics.md)           | Energy page metrics: attribution, cost/CO2, forecast accuracy                |
| [Discovery Hints](docs/agents/discovery-hints.md)             | Vendor registry update, template `discovery` hints                           |

Typical combinations: charger = hardware-integrations + core-domain; meter = hardware-integrations + plugin-system; vehicle = hardware-integrations (+ provider-auth for cloud login); UI = web-ui-api (+ charts); API endpoint = web-ui-api + core-domain; auth = api-security + web-ui-api; templates = plugin-system; bug = core-domain + the relevant topic file.

## Repository Map

- `main.go` entry point, embeds web assets and i18n. `cmd/` CLI commands and app setup.
- `core/` business logic: `loadpoint/`, `planner/`, `coordinator/`, `session/`, `vehicle/`, `soc/`, `site/`, `circuit/`, optimizer. `api/` shared types and interfaces.
- `server/` HTTP, WebSocket, MQTT, DB handlers, OpenAPI. `db/` persistence (SQLite).
- `charger/`, `meter/`, `vehicle/`, `tariff/`, `hems/`, `messenger/` device and service integrations. `plugin/` protocol plugins. `templates/definition/` YAML device templates.
- `assets/js/` Vue app (`views/`, `components/`, `composables/`, `types/`, `utils/`), `assets/css/`, `i18n/`. `tests/` Playwright specs plus `.evcc.yaml` fixtures.

## Technical Standards

These are the criteria design and review are held against.

### Go

- Idiomatic Go, `gofmt`/`gci` imports, early returns, small focused interfaces, composition over inheritance. Minimal external dependencies.
- Handle every error; wrap with context (`fmt.Errorf("...: %w", err)`), check with `errors.Is`/`errors.As`. `backoff.Permanent(err)` for non-retryable errors.
- `context.Context` first parameter for I/O and long-running work; propagate it into goroutines, honor `ctx.Done()`.
- Never block the control loop. Guard shared state with the existing mutex/primitives.
- Validate numeric input from devices: reject `NaN` and `Inf`.
- Assert interface conformance: `var _ api.X = (*T)(nil)`. Use `implement.Has`/`May` only for _conditional_ capabilities; unconditional ones are plain methods.
- New device implementations start from `_blueprint.go`. Prefer a plugin-based template over new Go code when possible.
- Generated code (`go:generate`: enumer, decorators, mockgen) is regenerated, never edited.
- Tests: `testing` + `testify` (`require` for setup, `assert` for checks), table-driven, `gomock` for interfaces. Cover success and failure.
- Linters: `golangci-lint` (govet, staticcheck, gci, ...) and `go tool modernize`; both must pass.

### Frontend (Vue 3 + TypeScript)

- TypeScript for all new code; Vue Options API; typed props, data, and API responses.
- Only top-level views read the store. Sub-components are stateless: props down, events up. Global state via `reactive()`; no Vuex/Pinia. `window.app` only for `raise()`, `setOffline()`/`setOnline()`, `clear()`.
- Prefer named computed properties over inline template expressions. Component methods as `const` arrow functions. Use the configured Axios instance.
- Accessibility: keyboard handlers, `aria-label`, `tabindex` where needed.
- Styling: CSS custom properties, Bootstrap utilities or scoped CSS, no inline `style` except truly dynamic values. Breakpoints via `assets/css/breakpoints.css` custom media queries, mobile-first.
- i18n: `$t()` for all user-facing strings; add keys to `i18n/en.json` and `i18n/de.json`; namespace `{section}.{component}.{purpose}`; placeholders for dynamic values; separators and trailing punctuation stay in the template. Check `i18n/glossary/` for terminology.
- Tooling is Vite+ (`vp`): oxlint, oxfmt, `vue-tsc`. Charts follow `docs/agents/charts.md`.

### Playwright

- Specs in `tests/*.spec.ts` with a dedicated `.evcc.yaml` per scenario; helpers in `tests/utils.ts`, binary handling in `tests/evcc.ts`; `test.afterEach(stop)`.
- Semantic selectors only (`getByRole`, `getByLabel`, `getByText`); `data-testid` as a last resort; no `.locator()`, class, or id selectors.
- No fixed timeouts, no `waitForLoadState("networkidle")`: wait on element state. Short, concrete test names.

### Templates and Configuration

- Templates in `templates/definition/<type>/`. Do not restate params already defined in `util/templates/defaults.yaml`; reference by `name`. Run `make docs` after changes.
- YAML config, SQLite with migrations for persistence. Clear validation messages for invalid config.
- Before coding against a third-party protocol library (eebus, ocpp-go, modbus/SunSpec), read its current upstream docs instead of relying on memory.

### Security and Performance

- Validate and sanitize all external input. TLS for external integrations. Respect the auth tiers in `docs/agents/api-security.md`.
- Never log passwords, tokens, or personal data.
- Index DB queries, bound caches, reuse connections.

## Conventions

- Language: English everywhere (code, comments, commits, PRs, issues).
- No em dashes in comments, commits, or docs. Project name is `evcc`, always lowercase. Acronyms uppercase in prose: OCPP, MQTT, HEMS, SoC. Mention `evcc` in user-facing strings only when needed.
- Comments: default to none. Only the non-obvious _why_, one line. No references to tasks, PRs, or callers. Exported Go identifiers follow godoc (`// Name does X`).
- Commit subjects: `Component: short description`, no trailing period; sub-scope in parens (`Meter (Home Assistant): ...`). `chore:`/`fix:`/`docs:` only for non-feature changes. Commit messages carry no AI footer and no `Co-Authored-By` trailer.
- Keep changes focused: one topic per PR.

### Pull Requests

No headlines, concise, in this order:

1. References first line (`fixes #1123`, `replaces #222`). Skip only for trivial fixes.
2. Intro: one or a few sentences on what and why.
3. Bullets: most significant changes and user-facing implications first.
4. `**TODO**` checklist only if open points remain.

No file paths, line numbers, or diff listings. No testing checklists. Never state that build, vet, race tests, or gofmt pass; they are a given.

### AI Attribution

Every PR description, issue body, review, and comment written by an agent ends with:

```
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

Never present agent work as human review.

## Working Agreements

- Ask before destructive or shared-state actions: force-push, `reset --hard`, deleting branches, posting to GitHub, modifying CI.
- Do not add features, error handling, or abstractions beyond the approved design.
- Prefer navigating Go code with go-to-definition and find-references over text search.
- Report honestly: what was run, what passed, what was not verified.
