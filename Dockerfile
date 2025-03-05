# ---- Stage 1: Build the binary and set up chroot ----
    FROM golang:1.23-bookworm AS builder

    # Install necessary tools for chroot and compilation
    RUN apt-get update && \
        apt-get install -y sudo debootstrap schroot g++ && \
        rm -rf /var/lib/apt/lists/*

    # Set working directory
    WORKDIR /app

    # Copy go mod files and install dependencies
    COPY go.mod go.sum ./
    RUN go mod download

    # Copy the application source code
    COPY . .

    # Make necessary scripts executable
    RUN chmod +x chroot_init_script.sh run_tests.sh

    # Run the chroot setup script (this initializes the chroot environment)
    RUN sudo ./chroot_init_script.sh

    # Build the Go application
    RUN CGO_ENABLED=0 GOOS=linux go build -o /worker-service ./cmd/main.go

    # ---- Stage 2: Create final image with chroot and necessary tools ----
    FROM debian:bookworm-slim

    # Install necessary dependencies, including g++ and chroot utilities
    RUN apt-get update && \
        apt-get install -y g++ schroot && \
        rm -rf /var/lib/apt/lists/*

    # Set working directory
    WORKDIR /app

    # Copy only the compiled binary from the builder stage
    COPY --from=builder /worker-service /app/worker-service

    # Copy the chroot environment from the builder stage
    COPY --from=builder /tmp/chroot /tmp/chroot

    # Set entrypoint for the container
    ENTRYPOINT [ "/app/worker-service" ]
