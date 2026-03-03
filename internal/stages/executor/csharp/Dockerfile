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

# create runner user same as other images
RUN groupadd -g 1000 runner && useradd -u 1000 -g 1000 -M -s /usr/sbin/nologin runner
USER runner
WORKDIR /tmp
