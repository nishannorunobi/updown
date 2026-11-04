# Dockerfile — updown's DEV image: the Go toolchain lives here, never on the host PC.
# The project is bind-mounted at /updown (docker-compose.yml), so edits on the host are
# built and run inside the container. Nothing is baked in: rebuild only when this file
# or GO_IMAGE changes.
ARG GO_IMAGE=golang:1.25-bookworm
FROM $GO_IMAGE

ARG CONTAINER_WORKDIR=/updown
WORKDIR $CONTAINER_WORKDIR

# Offline builds:
#   GOTOOLCHAIN=local  never download another Go toolchain
#   GOPROXY=off        never fetch modules from the internet — dependencies are vendored
#                      into the repo (go mod vendor) so a build needs no network
#   CGO_ENABLED=0      pure-Go, statically linked binaries: nothing needed on the customer PC
# GOPATH / GOCACHE point into /gocache (bind-mounted from mountspace), writable by the
# host user the container runs as.
ENV GOTOOLCHAIN=local \
    GOPROXY=off \
    CGO_ENABLED=0 \
    GOPATH=/gocache/gopath \
    GOCACHE=/gocache/build \
    HOME=/tmp \
    TZ=UTC

# Fluent Bit v5 from its official, signed Debian repository — pinned (LOGAN-29); the agent starts it
ARG FLUENT_BIT_VERSION=5.1.4
RUN apt-get update -qq && apt-get install -y --no-install-recommends curl gnupg ca-certificates \
 && curl -fsSL https://packages.fluentbit.io/fluentbit.key | gpg --dearmor -o /usr/share/keyrings/fluentbit.gpg \
 && . /etc/os-release \
 && echo "deb [signed-by=/usr/share/keyrings/fluentbit.gpg] https://packages.fluentbit.io/debian/${VERSION_CODENAME} ${VERSION_CODENAME} main" \
      > /etc/apt/sources.list.d/fluentbit.list \
 && apt-get update -qq && apt-get install -y --no-install-recommends fluent-bit=${FLUENT_BIT_VERSION} \
 && ln -sf /opt/fluent-bit/bin/fluent-bit /usr/local/bin/fluent-bit && rm -rf /var/lib/apt/lists/*

# A dev container stays up; work runs in it through host_scripts/exec.sh / shell.sh.
CMD ["sleep", "infinity"]
