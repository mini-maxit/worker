# Default to 3.12
ARG PYTHON_VERSION=3.12

FROM python:3.10-slim-trixie AS python310
FROM python:3.11-slim-trixie AS python311
FROM python:3.12-slim-trixie AS python312

FROM python${PYTHON_VERSION//./} AS runtime

ARG DEBIAN_FRONTEND=noninteractive

# upgrade pulls security fixes the base image tag doesn't have yet
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    ca-certificates time \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Solutions only need the interpreter (containers have no network), so drop the packaging tools:
# setuptools/wheel bundle vulnerable jaraco.context and wheel, pip vendors an old setuptools pkg_resources.
RUN (pip uninstall -y setuptools wheel 2>/dev/null || true) && pip uninstall -y pip

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

RUN groupadd -g 1000 runner && useradd -u 1000 -g 1000 -M -s /usr/sbin/nologin runner
USER runner
WORKDIR /tmp
