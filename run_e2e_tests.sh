#!/bin/bash

set -e

SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Detect platform and configure DOCKER_HOST if needed
if [[ "$OSTYPE" == "darwin"* ]]; then
    # macOS
    if [ -z "$DOCKER_HOST" ]; then
        DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
        export DOCKER_HOST
        echo "[*] Detected macOS. Setting DOCKER_HOST=$DOCKER_HOST"
    fi
elif [[ "$OSTYPE" == "linux-gnu"* ]]; then
    # Linux
    if [ -z "$DOCKER_HOST" ]; then
        DOCKER_HOST="unix:///var/run/docker.sock"
        export DOCKER_HOST
        echo "[*] Detected Linux. Setting DOCKER_HOST=$DOCKER_HOST"
    fi
elif [[ "$OSTYPE" == "msys" || "$OSTYPE" == "cygwin" ]]; then
    # Windows
    echo "[*] Detected Windows. Using Docker Desktop configuration."
fi

# Check if Docker is accessible
if ! docker ps &>/dev/null; then
    echo "[ERROR] Docker daemon is not accessible at $DOCKER_HOST"
    exit 1
fi

echo "[OK] Docker daemon is accessible"
echo ""

# Start required services
echo "[*] Starting required services (RabbitMQ, File Storage, Worker)..."
cd "$SCRIPT_DIR"

docker-compose up -d

# Wait for RabbitMQ to be ready
echo "[*] Waiting for RabbitMQ to be ready..."
max_attempts=30
attempt=0
until docker exec rabbitmq rabbitmqctl status &>/dev/null; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
        echo "[ERROR] RabbitMQ failed to start after $max_attempts attempts"
        docker-compose down
        exit 1
    fi
    echo "Waiting for RabbitMQ... (attempt $attempt/$max_attempts)"
    sleep 1
done

echo "[OK] RabbitMQ is ready"
echo ""

# Wait for file storage to be ready
echo "[*] Waiting for file storage to be ready..."
attempt=0
until curl -s http://localhost:8888/health &>/dev/null || curl -s http://localhost:8888/ &>/dev/null; do
    attempt=$((attempt + 1))
    if [ $attempt -ge $max_attempts ]; then
        echo "[ERROR] File storage failed to start after $max_attempts attempts"
        docker-compose down
        exit 1
    fi
    echo "Waiting for file storage... (attempt $attempt/$max_attempts)"
    sleep 1
done

echo "[OK] File storage is ready"
echo ""

# Wait for worker to be ready
echo "[*] Waiting for worker to be ready..."
sleep 10  # Give worker some time to start and connect

echo "[OK] All services are ready"
echo ""

# Run end-to-end tests
echo "[*] Running end-to-end tests..."
echo ""

TEST_FLAGS="-v -tags=e2e -timeout 10m"

EXIT_CODE=0

echo "Running: go test $TEST_FLAGS ./tests/e2e/..."
echo ""

if ! go test $TEST_FLAGS ./tests/e2e/...; then
    EXIT_CODE=1
fi

echo ""

if [ $EXIT_CODE -eq 0 ]; then
    echo ""
    echo "[OK] All end-to-end tests passed!"
else
    echo ""
    echo "[ERROR] End-to-end tests failed with exit code $EXIT_CODE"
fi

# Cleanup: Stop all containers
echo ""
echo "[*] Stopping all containers..."
docker-compose down

exit $EXIT_CODE
