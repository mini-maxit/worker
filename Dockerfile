FROM golang:1.23-bookworm AS builder

RUN apt-get update && \
    apt-get install -y sudo debootstrap schroot && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./cmd/main.go

FROM debian:bookworm-slim

RUN apt-get update && \
apt-get install -y \
    docker.io \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /worker-service /app/worker-service
COPY entrypoint.sh /app/entrypoint.sh

RUN chmod +x /app/entrypoint.sh

ENV DOCKER_HOST=unix:///var/run/docker.sock

ENTRYPOINT [ "/app/entrypoint.sh" ]
