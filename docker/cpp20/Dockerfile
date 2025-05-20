# docker/cpp20/Dockerfile
FROM debian:bookworm-slim

# avoid prompts, install only what we need
ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      g++-12         \
      ca-certificates \
 && rm -rf /var/lib/apt/lists/*

# Make gcc / g++ point to version 12
RUN update-alternatives --install /usr/bin/gcc  gcc  /usr/bin/gcc-12  100 \
 && update-alternatives --install /usr/bin/g++  g++  /usr/bin/g++-12  100

# Copy in your batch‐runner script
COPY docker/run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

# Create an unprivileged user
RUN useradd --create-home --shell /bin/bash runner

USER runner
WORKDIR /workspace
