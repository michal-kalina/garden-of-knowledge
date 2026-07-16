# Multi-stage build: one Dockerfile builds both (api & worker),
# docker-compose chooses target.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -o /out/worker ./cmd/worker

FROM alpine:latest AS api
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/api /usr/local/bin/api
EXPOSE 8080
ENTRYPOINT ["api"]

FROM alpine:latest AS worker
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/worker /usr/local/bin/worker
ENTRYPOINT ["worker"]
