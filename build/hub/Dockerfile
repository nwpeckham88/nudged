FROM node:20-alpine AS web-builder
WORKDIR /src
COPY web/package.json web/package-lock.json ./web/
RUN cd web && npm ci
COPY web/ ./web/
# This will output to ../internal/webui/dist relative to web/, so /src/internal/webui/dist
RUN cd web && npm run build

FROM golang:1.24-alpine AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy the built web assets from the web-builder stage to the expected location for embedding
COPY --from=web-builder /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/nudged-hub ./cmd/hub

FROM alpine:3.19
RUN apk add --no-cache ca-certificates curl
COPY --from=go-builder /out/nudged-hub /usr/local/bin/nudged-hub
EXPOSE 8080
HEALTHCHECK --interval=5s --timeout=3s CMD curl -f http://localhost:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/nudged-hub", "--serve"]
