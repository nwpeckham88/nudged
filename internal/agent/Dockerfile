# Multi-stage Dockerfile for the Nudged Agent
FROM golang:1.20 AS builder
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/nudged-agent ./cmd/nudged-agent

FROM gcr.io/distroless/static-debian11
COPY --from=builder /out/nudged-agent /nudged-agent
ENTRYPOINT ["/nudged-agent"]
