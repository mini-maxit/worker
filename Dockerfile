    FROM golang:1.23-bookworm AS builder

    RUN apt-get update && \
        apt-get install -y sudo debootstrap schroot g++ && \
        rm -rf /var/lib/apt/lists/*

    WORKDIR /app

    COPY go.mod go.sum ./
    RUN go mod download

    COPY . .

    RUN chmod +x chroot_init_script.sh

    RUN sudo ./chroot_init_script.sh

    RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./cmd/main.go

    FROM debian:bookworm-slim

    RUN apt-get update && \
        apt-get install -y g++ schroot && \
        rm -rf /var/lib/apt/lists/*

    WORKDIR /app

    COPY --from=builder /worker-service /app/worker-service

    COPY --from=builder /tmp/chroot /tmp/chroot

    ENTRYPOINT [ "/app/worker-service" ]
