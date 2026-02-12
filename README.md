# Nudged

Monorepo for Nudged (Hub, Agent, Svelte dashboard).

Build everything:

```sh
make
```

Build individual components:

```sh
make build-web        # builds web/build (Svelte)
make build-hub        # builds bin/nudged-hub
make build-agent      # builds bin/nudged-agent
make build-webserver  # builds bin/webserver (embeds web/build)
```

Run tests:

```sh
make test
```

Next steps: split into `cmd/hub` and `cmd/agent`, implement WebSocket registration and Docker control.
