FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/sim-service ./cmd/sim-service

RUN CGO_ENABLED=0 GOOS=linux go build -o /sim-service ./cmd/sim-service

FROM alpine:latest
WORKDIR /
COPY --from=builder /sim-service /sim-service
ENV PORT=80
ENTRYPOINT ["/sim-service"]
