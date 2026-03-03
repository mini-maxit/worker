# multi-stage build for C# runtime images supporting versions 8, 9, 10
# Use SDK images which include csc compiler needed for compilation
ARG DOTNET_VERSION=8.0

FROM mcr.microsoft.com/dotnet/sdk:8.0 AS sdk8
FROM mcr.microsoft.com/dotnet/sdk:9.0 AS sdk9
FROM mcr.microsoft.com/dotnet/sdk:10.0 AS sdk10

# select SDK based on version (extract major version from DOTNET_VERSION)
FROM sdk${DOTNET_VERSION%%.*} AS runtime

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates time \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

# Provide a small `csc` wrapper that uses `dotnet` to execute the Roslyn csc.dll
# placed under the SDK path. Run this as root (before switching to `runner`) so
# the wrapper is writable to `/usr/local/bin`.
RUN set -eux; \
    printf '%s\n' '#!/bin/sh' \
      'for f in /usr/share/dotnet/sdk/*/Roslyn/bincore/csc.dll; do' \
      '  if [ -f "$f" ]; then' \
      '    exec dotnet "$f" "$@"' \
      '  fi' \
      'done' \
      'echo "csc: could not find csc.dll" >&2' \
      'exit 127' > /usr/local/bin/csc; \
    chmod +x /usr/local/bin/csc

# create runner user same as other images (idempotent safe creation)
# If GID/UID 1000 already exists in base image, fall back to creating a group/user without fixed IDs.
RUN set -eux; \
    # Ensure a group with GID 1000 exists and is named 'runner'. If a different group uses GID 1000, rename it.
    if getent group 1000 >/dev/null 2>&1; then \
        existing_group=$(getent group 1000 | cut -d: -f1); \
        if [ "$existing_group" != "runner" ]; then \
            groupmod -n runner "$existing_group" || true; \
        fi; \
    else \
        groupadd -g 1000 runner; \
    fi; \
    # Ensure a user with UID 1000 exists and is named 'runner'. If a different user uses UID 1000, rename it.
    if getent passwd 1000 >/dev/null 2>&1; then \
        existing_user=$(getent passwd 1000 | cut -d: -f1); \
        if [ "$existing_user" != "runner" ]; then \
            usermod -l runner "$existing_user" || true; \
            # update home/group reference if needed
            usermod -g runner runner || true; \
        fi; \
    else \
        useradd -u 1000 -g runner -M -s /usr/sbin/nologin runner || true; \
    fi; \
    true
USER runner
WORKDIR /tmp
