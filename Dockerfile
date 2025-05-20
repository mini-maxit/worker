FROM golang:1.23-bookworm AS builder

    RUN apt-get update && \
        apt-get install -y sudo debootstrap schroot g++ && \
        rm -rf /var/lib/apt/lists/*

    WORKDIR /app

    COPY go.mod go.sum ./
    RUN go mod download

    COPY . .

    RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./cmd/main.go

    FROM debian:bookworm-slim

    RUN apt-get update && \
        apt-get install -y \
        g++ \
        schroot \
        docker.io \
        && rm -rf /var/lib/apt/lists/*

    WORKDIR /app

    COPY --from=builder /worker-service /app/worker-service

    ENV DOCKER_HOST=unix:///var/run/docker.sock

    ENTRYPOINT [ "/app/worker-service" ]
