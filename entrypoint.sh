#!/bin/bash
set -e

RUNTIME_IMAGE_BASE="ghcr.io/mini-maxit/runtime"

RUNTIME_LANG_VERSIONS=(
    "cpp:latest"
    "python-3.12:latest"
    "python-3.11:latest"
    "python-3.10:latest"
)

for lang_version in "${RUNTIME_LANG_VERSIONS[@]}"; do
    image="${RUNTIME_IMAGE_BASE}-${lang_version}"
    echo "Pulling $image..."
    if docker pull "$image"; then
        echo "Successfully pulled $image"
    else
        echo "Warning: Failed to pull $image, will be pulled on-demand"
    fi
done

exec /app/worker-service "$@"
