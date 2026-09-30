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

# A dev container stays up; work runs in it through host_scripts/exec.sh / shell.sh.
CMD ["sleep", "infinity"]
