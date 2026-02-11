FROM golang:1.21-alpine AS builder
WORKDIR /src

RUN apk add --no-cache git nodejs npm

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# build web UI so it can be embedded (SvelteKit writes to `build` with adapter-static)
RUN cd web && npm ci --silent && npm run build --silent && mkdir -p ../internal/webui/dist && cp -r build/* ../internal/webui/dist/ || true

# Build a static-ish binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/nudged ./cmd/nudged

FROM alpine:3.18
RUN apk add --no-cache ca-certificates
COPY --from=builder /out/nudged /usr/local/bin/nudged
# copy built web UI into the final image so the binary can serve it from disk
COPY --from=builder /src/web/build /internal/webui/dist

EXPOSE 8080
HEALTHCHECK CMD wget -qO- --timeout=2 --tries=1 http://localhost:8080/health || exit 1

ENTRYPOINT ["/usr/local/bin/nudged","--serve"]
