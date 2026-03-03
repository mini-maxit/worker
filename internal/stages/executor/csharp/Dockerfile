ARG DOTNET_VERSION=9.0
FROM mcr.microsoft.com/dotnet/sdk:${DOTNET_VERSION} AS runtime

ARG DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
  ca-certificates time \
  mono-devel \
  && ln -sf /usr/bin/mono-csc /usr/bin/csc \
  && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

# create a non-root `runner` user (idempotent)
RUN set -eux; \
  if getent group 1000 >/dev/null 2>&1; then \
    existing_group=$(getent group 1000 | cut -d: -f1); \
    if [ "$existing_group" != "runner" ]; then \
      groupmod -n runner "$existing_group" || true; \
    fi; \
  else \
    groupadd -g 1000 runner; \
  fi; \
  if getent passwd 1000 >/dev/null 2>&1; then \
    existing_user=$(getent passwd 1000 | cut -d: -f1); \
    if [ "$existing_user" != "runner" ]; then \
      usermod -l runner "$existing_user" || true; \
      usermod -g runner runner || true; \
    fi; \
  else \
    useradd -u 1000 -g runner -M -s /usr/sbin/nologin runner || true; \
  fi

USER runner
WORKDIR /tmp
