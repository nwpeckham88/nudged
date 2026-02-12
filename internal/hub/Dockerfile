# Multi-stage Dockerfile for the Nudged Hub
FROM golang:1.20 AS builder
WORKDIR /src

# Download deps first
COPY go.mod go.sum ./
RUN go mod download

# Copy the project and build the hub binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /out/nudged-hub ./cmd/nudged-hub

FROM gcr.io/distroless/static-debian11
COPY --from=builder /out/nudged-hub /nudged-hub
EXPOSE 8080
ENTRYPOINT ["/nudged-hub"]
