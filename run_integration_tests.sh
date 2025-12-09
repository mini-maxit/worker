#!/bin/bash

set -e

TEST_SUITES=(
    "./internal/docker/..."
)

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

# Run the tests
echo "[*] Running integration tests..."
cd "$SCRIPT_DIR"

TEST_FLAGS="-v -tags=integration -timeout 5m"

# Print test suites to be executed
echo "Test suites to be executed (${#TEST_SUITES[@]} total):"
for suite in "${TEST_SUITES[@]}"; do
    echo "  - $suite"
done
echo ""

EXIT_CODE=0

# Run each test suite
for suite in "${TEST_SUITES[@]}"; do
    echo "Running: go test $TEST_FLAGS $suite"
    echo ""

    if ! go test $TEST_FLAGS "$suite"; then
        EXIT_CODE=1
    fi

    echo ""
done

if [ $EXIT_CODE -eq 0 ]; then
    echo ""
    echo "[OK] All tests passed!"
else
    echo ""
    echo "[ERROR] Tests failed with exit code $EXIT_CODE"
fi

exit $EXIT_CODE
