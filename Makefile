.PHONY: all build-web build-hub build-agent build-webserver test fmt

all: build-web build-hub build-agent build-webserver

build-web:
	cd web && npm ci && npm run build

build-hub:
	go build -v -o bin/nudged-hub ./cmd/nudged-hub

build-agent:
	go build -v -o bin/nudged-agent ./cmd/nudged-agent

build-webserver:
	go build -v -o bin/webserver ./cmd/webserver

test:
	go test ./...

fmt:
	gofmt -w .
