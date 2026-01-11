FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
    libstdc++6 ca-certificates time \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

RUN groupadd -g 1000 runner && useradd -u 1000 -g 1000 -M -s /usr/sbin/nologin runner
USER runner
WORKDIR /tmp
