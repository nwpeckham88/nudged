FROM golang:1.21-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build a static-ish binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/nudged ./cmd/nudged

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/nudged /usr/local/bin/nudged

EXPOSE 8080
HEALTHCHECK CMD wget -qO- --timeout=2 --tries=1 http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/nudged","--serve"]
