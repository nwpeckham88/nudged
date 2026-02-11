FROM golang:1.21-alpine AS build
WORKDIR /src
COPY main.go ./
RUN go build -ldflags='-s -w' -o /bin/test-service ./main.go

FROM alpine:3.18
COPY --from=build /bin/test-service /usr/local/bin/test-service
EXPOSE 8082
ENTRYPOINT ["/usr/local/bin/test-service"]
