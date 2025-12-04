# Default to 3.12
ARG PYTHON_VERSION=3.12

FROM python:3.8-slim AS python38
FROM python:3.9-slim AS python39
FROM python:3.10-slim AS python310
FROM python:3.11-slim AS python311
FROM python:3.12-slim AS python312

FROM python${PYTHON_VERSION//./} AS runtime

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

RUN useradd -M -s /usr/sbin/nologin runner
USER runner
WORKDIR /tmp
