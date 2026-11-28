FROM golang:1.27.4-trixie AS builder

RUN apt-get update && \
    apt-get install -y sudo debootstrap schroot && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./cmd/main.go

FROM debian:trixie-slim

# upgrade pulls security fixes the base image tag doesn't have yet
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /worker-service /app/worker-service

ENV DOCKER_HOST=unix:///var/run/docker.sock

ENTRYPOINT [ "/app/worker-service" ]
