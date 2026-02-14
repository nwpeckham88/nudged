# Project Design Document: Nudged

**Version:** 1.4.0 · **Status:** Draft

## 1. Executive Summary

Nudged is a distributed, "hub-and-spoke" container orchestration tool for homelabs. It acts as a specialized reverse proxy that intercepts HTTP traffic to stopped containers, holds the connection, wakes the container on the appropriate remote host, and transparently proxies traffic once the service is ready.

It provides a "Serverless"/"Scale-to-Zero" experience for Docker, designed to operate over flat mesh networks (LAN/VPN) while mitigating split-tunneling and routing attack vectors.

## 2. Architecture Overview

Two binaries:

- Hub (`nudged-hub`): Control Plane & Ingress — requires a stable ingress (public URL).
- Agent (`nudged-agent`): Execution Plane — runs on each node.

### 2.1 The Hub (nudged-hub)

Role: brain — single ingress point.

Responsibilities:
- Ingress proxy: Accepts incoming HTTP requests (behind Caddy).
- State management: Registry of registered Agents and their Apps (SQLite backed).
- Splash screen: Serves "Waking Up" HTML while holding connections.
- Routing: Proxies traffic to the correct Agent IP once app is ready.

### 2.2 The Agent (nudged-agent)

Role: hands — runs on every node.

Responsibilities:
- Docker control: Start/stop containers via local Docker socket.
- Registration: Dial Hub via WebSocket to register capabilities/apps.
- Health checks: Local checks (e.g., curl localhost:3000) before signaling readiness.
- Interface binding: Listen only on the Netbird/VPN interface to prevent LAN leakage.
- Idle Management: Tracks request activity and stops containers after a timeout.

## 3. Communication Protocol

### 3.1 Control Plane (WebSocket)

Agent maintains a persistent WebSocket to the Hub so Hub can send commands without Agents needing a public IP.

- Endpoint: `ws://HUB_IP:8080/ws/register`
- Authentication: HMAC shared secret (API key) in headers.

Message flow (examples):
```
Agent -> Hub: REGISTER { "name": "node-parents", "apps": ["plex","request-arr"] }
Hub   -> Agent:  WAKE { "app": "plex" }
Agent -> Hub: STATUS { "app": "plex", "state": "STARTING" }
Agent -> Hub: STATUS { "app": "plex", "state": "READY", "port": 32400 }
```

### 3.2 Data Plane (HTTP Proxy)

Traffic flows Hub -> Agent.

- Hub uses Go's `httputil.ReverseProxy`.
- Target: IP of incoming WebSocket connection (Trust-the-Source model).
- Mechanism: Standard HTTP proxying over the VPN interface.

### 3.3 Network Security & Routing

To mitigate Route Injection Attacks:
- Interface binding: Agent binds HTTP listener to Netbird interface IP (e.g., `100.64.0.5`) — not `0.0.0.0`.
- Traffic encryption: Treat VPN as potentially hostile.

Strategies:
- V1: Shared secret tokens in Control Plane messages.
- V2: mTLS between Hub and Agent to prevent MITM.

## 4. Feature Specifications

### 4.1 Zero-Config Discovery (Docker Labels)

Agents scan the local Docker daemon for containers with labels; no agent config files required.

Labels:
```
nudged.enable=true        # Enlist the container
nudged.port=3000          # Internal port for health checks
nudged.name=plex          # Optional: custom Hub routing name (defaults to container name)
nudged.timeout=30m        # Optional: idle time before stopping (default: never)
nudged.capability=transcode # Optional: for future scheduling
```

### 4.2 The "Hold & Wake" Logic

Flow:
1. Incoming request: `plex.kn8design.com`.
2. Hub intercept: App is STOPPED or Agent returns 502/503.
3. Connection hold: Hub serves HTTP 200 `Content-Type: text/html` with a Splash Screen containing a WebSocket client that listens for updates.
4. Wake command: Hub sends `WAKE` to Agent.
5. Agent:
    - `docker start plex`
    - Loops `curl http://127.0.0.1:32400` (or TCP dial)
    - On success, sends `READY` to Hub.
6. Browser refresh: Splash Screen receives "Ready" via WebSocket and reloads.
7. Proxy: Hub proxies to `http://AGENT_IP:32400`.

### 4.3 Idle Timeout & Auto-Stop

Agent tracks the last time a request was proxied to an app.

- **Tracking**: Middleware in Agent's proxy server updates `LastActivity` timestamp on every request.
- **Enforcement**: Background goroutine checks every 1 minute.
- **Logic**: If `time.Now() - LastActivity > nudged.timeout`:
  1. Agent stops the container (`docker stop`).
  2. Agent sends `STATUS { "state": "STOPPED" }` to Hub (optional, mainly for UI updates).
- **Default**: If `nudged.timeout` is missing, auto-stop is disabled.

### 4.4 Resource Scheduling (Future "Mini-Swarm")

Hub can choose where to run tasks if multiple Agents advertise the same capability.

- Label: `nudged.capability=transcode`
- Request: `Run(transcode)`
- Logic: Hub queries Agents for CPU load (heartbeat) and picks the least loaded node.

## 5. Technical Stack

- Language: Go 1.22+
- Docker SDK: `github.com/docker/docker/client`
- WebSockets: `github.com/gorilla/websocket` or `nhooyr.io/websocket`
- Routing/HTTP: Go stdlib `net/http`
- Configuration: Environment variables (`NUDGED_HUB_SECRET`, `NUDGED_HUB_URL`)
- Networking: Agents run in host mode or bind directly to `tun0` to bridge Docker <-> VPN
- Logging: `log/slog` (structured JSON)
- Persistence: `modernc.org/sqlite` (Hub registry)

## 6. Implementation Roadmap

Phase 1 — Local Prototype (Monolith) [Completed]
- Single binary acting as Hub+Agent on laptop.
- Talks to local Docker socket.
- Implements Hold & Wake proxy.
- Goal: validate Splash Screen UX.

Phase 2 — Split (Hub & Agent) [Completed]
- Split into `cmd/hub` and `cmd/agent`.
- Implement WebSocket registration with shared-secret auth.
- Test on LAN (Hub on desktop, Agent on laptop).
- Goal: remote orchestration.

Phase 3 — Mesh & Security [Completed]
- Deploy Hub to cloud/primary server behind Caddy.
- Deploy Agents on remote machines over Netbird.
- Implement explicit interface binding (`-bind-addr`).
- Goal: zero-config remote management.

Phase 4 — Hardening & Persistence
- Add SQLite persistence for Hub registry. [Completed]
- Add structured logging. [Completed]
- Add "Stop" functionality (idle timeout). [Completed]
- Add metrics (Prometheus). [Next]

## 7. Security Considerations

- Hub: behind Caddy/Nginx handling TLS.
- Agent: authenticate to Hub with shared secret (HMAC) to prevent unauthorized registration.
- Network trust: V1 relies on VPN security; Hub assumes valid Netbird IPs are trustworthy.
  - Mitigation: Agents bind only to Netbird interface IP.

## 8. Failure Modes

- Agent offline: Hub removes Agent from registry (memory) but keeps in DB; returns `503 Service Unavailable` (or custom error page).
- App crash: If `docker start` fails, Agent reports `ERROR`; Hub displays error on Splash Screen.
- Stuck waking: Splash Screen times out after 60s with a "Retry" button.
- Route injection: If routing is compromised, connection fails (TLS error) or Agent rejects requests (invalid HMAC), protecting data.

## 9. Data Persistence

The Hub uses an SQLite database (`nudged.db`) to persist the registry.
- **Schema**: `agents` table storing ID, Name, Addr, LastSeen, and Apps (JSON).
- **Behavior**: On startup, loads known agents. On registration, upserts agent state.

## 10. API Specification

### 10.1 Hub API (Public/Internal)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| GET | `/health` | None | Health check |
| GET | `/agents` | None* | List connected agents (*Auth in future) |
| GET | `/apps` | None* | List known apps |
| POST | `/wake?app=NAME` | None* | Manually wake an app |
| GET | `/metrics` | None | Prometheus metrics |
| GET | `/ws/notify?app=NAME`| None | WebSocket for splash screen updates |
| GET | `/ws/register` | Secret | WebSocket for Agent registration |

### 10.2 Agent API (Internal Proxy)

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| ANY | `/*` | None | Proxied traffic to container |
| GET | `/health` | None | Agent health check |
| GET | `/metrics` | None | Prometheus metrics |

## 11. Observability Strategy

**Logging:**
- Uses `slog` for structured JSON logging.
- Fields: `component` (hub/agent), `trace_id`, `app`, `agent_id`, `error`.

**Metrics (Planned):**
- Uses `github.com/prometheus/client_golang/prometheus`.
- Exposed at `/metrics` on both Hub and Agent.

**Hub Metrics:**
- `nudged_hub_connected_agents` (Gauge): Number of currently connected agents.
- `nudged_hub_requests_total` (Counter): Total HTTP requests handled by Hub.
- `nudged_hub_wake_requests_total` (Counter): Total wake requests triggered.

**Agent Metrics:**
- `nudged_agent_running_apps` (Gauge): Number of apps currently in RUNNING state.
- `nudged_agent_proxied_requests_total` (Counter): Total requests proxied to containers.
- `nudged_agent_container_starts_total` (Counter): Total successful container starts.
