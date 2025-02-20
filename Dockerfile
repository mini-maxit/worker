# Start from a specific version of the Go image
FROM golang:1.23-bookworm

# Install sudo and tools needed for chroot
RUN apt-get update && \
    apt-get install -y sudo debootstrap schroot && \
    rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /app

# Copy go mod files and install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application files
COPY . .

# Make necessary scripts executable
RUN chmod +x wait-for-it.sh
RUN chmod +x app_entry_script.sh
RUN chmod +x chroot_init_script.sh
RUN chmod +x run_tests.sh

RUN sudo ./chroot_init_script.sh

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux go build -o ./bin/worker-service ./cmd/main.go

# Set entrypoint for the container
ENTRYPOINT ["./app_entry_script.sh"]
