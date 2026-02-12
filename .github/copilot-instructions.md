<!-- Copilot / AI agent instructions for contributors working on Nudged -->

# Nudged — Copilot Instructions (concise)

Purpose: provide AI coding agents the minimal, precise context to be productive in this repository.

## Project Guidelines

### Code Style
- Go: use `gofmt`/`go vet`. Follow existing patterns in `pkg/` and `cmd/`.
- Frontend: follow `web/` configs (`svelte.config.js`, `tsconfig.json`, `web/package.json`). Run `npm run lint` and `npm run dev` for checks.

### Architecture
- Hub (control/ingress): `cmd/nudged-hub/main.go` (run with `-serve`). Core HTTP and websocket logic is in `pkg/server/server.go`.
- Agent: `cmd/nudged-agent/main.go` — agents open a control WebSocket to `/ws/register` and advertise `apps`.
- Pub/sub: use `internal/hub/hub.go` for cross-component events and in-memory testing.

### Build & Test (commands to run)
- Build hub: `go build ./cmd/nudged-hub`
- Run hub (local): `./nudged-hub -serve` or `go run ./cmd/nudged-hub -serve`
- Build agent: `go build ./cmd/nudged-agent`
- Run tests: `go test ./...`
- Frontend: `cd web && npm install` then `npm run dev` / `npm run build` / `npm run preview`

### Project Conventions (concrete)
- Agent-provided `addr` is authoritative; do not rewrite it without updating `docs/NUDGED_DEISGN.md`.
- Registry state is in-memory. Treat `pkg/server/Registry` as ephemeral for tests and dev.
- Preserve the wake/splash proxy flow: failures trigger a `wake` message and a splash page (see `pkg/server/server.go`).

### Integration Points
- HTTP endpoints: `POST /agents`, `GET /apps`, and proxy root at `GET /` (host-based routing).
- WebSockets: `/ws/register` (agent control) and `/ws/notify?app=NAME` (splash notifications).
- Frontend tooling and MCP guidance: `web/AGENTS.md`.

### Security Notes
- `NUDGED_HUB_SECRET` secures `/ws/register`; hub expects header `X-Nudged-Secret`.
- Treat agent `addr` and control messages as sensitive — any change in trust model requires documentation updates and review.

## Quick References (where to look)
- Core server: `pkg/server/server.go`
- Pub/sub and tests: `internal/hub/hub.go`, `internal/hub/hub_test.go`
- Design & rationale: `docs/NUDGED_DEISGN.md`
- Frontend MCP rules: `web/AGENTS.md`

If you'd like more examples, unit-test snippets, or specific grep/search targets to include, tell me which section to expand.
<!-- Copilot / AI agent instructions for contributors working on Nudged -->

# Nudged — Copilot Instructions (concise)

Purpose: give AI coding agents the minimal, precise context to be productive in this repository.

- **Big picture**: Nudged is a hub-and-agent system (see `docs/NUDGED_DEISGN.md`). The Hub (`cmd/nudged`) is the control/ingress plane; Agents connect to the Hub over a persistent WebSocket and advertise `Apps`. The Hub reverse-proxies HTTP to agent addresses and implements a "hold & wake" splash flow when a service is unavailable.

- **Primary Go entrypoints**: `cmd/nudged/main.go` (start with `-serve`). The server core is `pkg/server/server.go` which registers HTTP handlers and the WebSocket control-plane. The in-memory pub/sub helper is `internal/hub/hub.go`.

- **How to build & run (local dev)**:
  - Build: `go build ./cmd/nudged-hub`
  - Run: `./nudged-hub -serve` (binds to `:8080` by default) or `go run ./cmd/nudged-hub -serve`
  - Tests: `go test ./...`

- **Key HTTP endpoints and behaviors (see `pkg/server/server.go`)**:
  - `POST /agents` — register or update agent metadata (JSON `id`, `name`, `addr`, `apps`).
  - `GET /apps` — list apps → agents advertising them.
  - `GET /` — core reverse-proxy: determines `app` from the request host (first subdomain) and proxies to a registered agent.
  - `/ws/register` — control-plane WebSocket where Agents send an initial identity JSON and later status messages.
  - `/ws/notify?app=NAME` — notification WebSocket used by the splash HTML to listen for app readiness messages.
  - `NUDGED_HUB_SECRET` — environment variable used to protect `/ws/register` (header `X-Nudged-Secret`).

- **Control-plane messages & topics** (examples from code/design):
  - Agent -> Hub: REGISTER (first message) `{ "id": "node-1", "name":"node-1","addr":"100.64.0.5:80","apps":["plex"] }`
  - Hub -> Agent: WAKE `{ "type":"wake","app":"plex" }`
  - Agent -> Hub: STATUS `{ "type":"status","app":"plex","state":"READY","port":32400 }`
  - Hub publishes to the in-memory hub with topics like `app:plex` and `wake:plex`.

- **Wake & Splash flow**: When proxying fails, the server sends a `wake` message (via Agent WebSocket) and returns a small splash HTML that opens a `/ws/notify?app=...` socket. When the Agent reports `state: READY` the splash reloads — see the inline splash HTML in `pkg/server/server.go`.

- **Frontend (web)**:
  - SvelteKit app lives in `web/` and is embedded/served by `internal/webui` (registered at `/ui/`).
  - Common commands: `npm run dev` (local), `npm run build` (production), `npm run preview`.
  - Frontend dev dependencies and scripts are in `web/package.json`.

- **Conventions & patterns to preserve**:
  - Agent identity is authoritative for the `Addr` used by the Hub (trust-the-source on VPN). Do not change that without updating security considerations in `docs/NUDGED_DEISGN.md`.
  - The project uses a small in-memory registry (not a DB). Treat `pkg/server/Registry` as ephemeral state for most dev work.
  - Pub/sub is implemented using `internal/hub/Hub` — prefer publishing events there for cross-component notifications in tests and local logic.

- **Where to look for example code / quick answers**:
  - startup and flags: `cmd/nudged-hub/main.go`
  - server handlers, reverse proxy, websockets: `pkg/server/server.go`
  - pub/sub helper: `internal/hub/hub.go`
  - design rationale & labels for Docker: `docs/NUDGED_DEISGN.md`
  - frontend guidance / Svelte MCP notes: `web/AGENTS.md` and `web/package.json`

- **Common dev tasks (examples to follow precisely)**:
  - To reproduce the "wake" flow locally: run the server (`-serve`), register a mock agent via `POST /agents` or open a WebSocket to `/ws/register` and send the initial identity payload. Then request `app.localhost` (or set Host header) to trigger proxy logic.
  - To debug agent lifecycle: inspect WebSocket messages in `pkg/server` goroutines and watch `hub.Publish` topics (use the `Subscribe` helper in `internal/hub`).

If anything in these notes is unclear or you'd like additional examples (curl snippets, unit-test pointers, or more frontend guidance), tell me which section to expand.

- **Frontend MCP notes (important)**: see `web/AGENTS.md` for Svelte MCP tool guidance. Key rules to preserve when working on frontend code:
  - Always run the `list-sections` MCP tool first when researching Svelte/SvelteKit docs.
  - Use `get-documentation` to fetch all relevant sections after identifying them.
  - Before sending or committing Svelte components, run `svelte-autofixer` (the project uses Svelte 5). Keep calling it until it reports no issues.
  - If you generate a Svelte example for the user, ask whether they want a Playground link before calling the playground generator.

- **Quick examples** (copy/paste to reproduce behaviors):
  - Register an agent (HTTP):
    ```sh
    curl -X POST http://localhost:8080/agents -H 'Content-Type: application/json' -d '{"id":"node-1","name":"node-1","addr":"100.64.0.5:80","apps":["plex"]}'
    ```
  - Send initial WebSocket identity to `/ws/register` (JS example):
    ```js
    const ws = new WebSocket('ws://localhost:8080/ws/register');
    ws.onopen = () => ws.send(JSON.stringify({id:'node-1',name:'node-1',addr:'100.64.0.5:80',apps:['plex']}));
    ```
  - Trigger a manual wake from the splash (server exposes `POST /wake?app=NAME` in splash JS):
    ```sh
    curl -X POST "http://localhost:8080/wake?app=plex"
    ```
  - Simulate a browser request for a specific app using `Host` header (useful for local testing):
    ```sh
    curl -H 'Host: plex.localhost' http://127.0.0.1:8080/
    ```
