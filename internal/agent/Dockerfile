FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/nudged-agent ./cmd/agent

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=builder /out/nudged-agent /usr/local/bin/nudged-agent
ENTRYPOINT ["/usr/local/bin/nudged-agent"]
