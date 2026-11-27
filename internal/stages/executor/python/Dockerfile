# Default to 3.12
ARG PYTHON_VERSION=3.12

FROM python:${PYTHON_VERSION}-slim-trixie AS runtime

# lets the worker remove the images it replaces (constants.RuntimeImageLabel)
LABEL org.mini-maxit.runtime="true"

ARG DEBIAN_FRONTEND=noninteractive

# upgrade pulls security fixes the base image tag doesn't have yet
RUN apt-get update && apt-get upgrade -y && apt-get install -y --no-install-recommends \
    ca-certificates time \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

# Solutions only need the interpreter (containers have no network), so drop the packaging tools:
# setuptools/wheel bundle vulnerable jaraco.context and wheel, pip vendors an old setuptools pkg_resources.
RUN pip uninstall -y setuptools wheel pip

COPY run_tests.sh /usr/local/bin/run_tests.sh
RUN chmod +x /usr/local/bin/run_tests.sh

RUN groupadd -g 1000 runner && useradd -u 1000 -g 1000 -M -s /usr/sbin/nologin runner
USER runner
WORKDIR /tmp
