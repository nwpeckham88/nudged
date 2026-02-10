.PHONY: build test
build:
	go build -v ./cmd/nudged

test:
	go test ./...
