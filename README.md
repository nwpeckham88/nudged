# Nudged

Scaffold for the Nudged project (Hub & Agent orchestration prototype).

Build:

```sh
go build ./cmd/nudged
```

Run tests:

```sh
go test ./...
```

Next steps: split into `cmd/hub` and `cmd/agent`, implement WebSocket registration and Docker control.
