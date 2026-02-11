Local Docker test
=================

Build and run the hub and a simple example agent for local testing.

Build images:

```bash
docker compose build
```

Start the stack:

```bash
docker compose up
```

The hub will be reachable at http://localhost:8080 and exposes a health endpoint at `/health` (also available at `/healthz`).

The Go example agent exposes a health endpoint at `http://<agent-host>:8081/health` and will connect to the hub and log registration and any messages it receives.

To stop:

```bash
docker compose down
```
