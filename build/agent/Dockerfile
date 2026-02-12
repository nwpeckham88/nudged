FROM golang:1.21-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# build the agent binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags='-s -w' -o /out/agent ./cmd/agent

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/agent /usr/local/bin/agent

ENV HUB_ADDR ws://hub:8080/ws/register

EXPOSE 8081
HEALTHCHECK CMD wget -qO- --timeout=2 --tries=1 http://localhost:8081/health || exit 1

ENTRYPOINT ["/usr/local/bin/agent"]
