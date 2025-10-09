ARG DOCKER_HUB

FROM --platform=${BUILDPLATFORM} ${DOCKER_HUB}library/golang:1.25.2-alpine AS builder

# Set build arguments
ARG TARGETPLATFORM

RUN apk update \
  && apk add --no-cache bash unzip zip nodejs npm \
  && GO_TASK_VERSION=3.44.1 \
  && TARGETARCH=$(uname -m) \
  && case ${TARGETPLATFORM} in \
    'linux/amd64') url="https://github.com/go-task/task/releases/download/v${GO_TASK_VERSION}/task_linux_amd64.tar.gz" ;; \
    'linux/arm64'*) url="https://github.com/go-task/task/releases/download/v${GO_TASK_VERSION}/task_linux_arm64.tar.gz" ;; \
    *) echo "Unsupported architecture: ${TARGETPLATFORM}" && exit 1 ;; \
  esac \
  && wget -O /tmp/task.tgz $url \
  && tar -xzf /tmp/task.tgz -C /usr/local/bin/

WORKDIR /app

COPY . ./

# Install npm dependencies
RUN npm install

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
	\
	# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
	go mod download

# Build the clients for all platforms and the application for the target architecture
RUN echo "Building for target platform: ${TARGETPLATFORM}" \
  && case ${TARGETPLATFORM} in \
    'linux/amd64') task build-amd64 ;; \
    'linux/arm64'*) task build-arm64 ;; \
    *) echo "Unsupported target platform: ${TARGETPLATFORM}" && exit 1 ;; \
  esac

FROM ${DOCKER_HUB}library/alpine:3.22

ARG VERSION=0.0.1

# Upgrade to the latest versions
RUN apk update \
  && apk upgrade \
  && apk add bash unzip

# Copy client files in
COPY --from=builder /app/bin/*.zip /srv/

# Copy the main executable
COPY --from=builder /app/bin/knot /usr/local/bin/knot

# Add a user to run the process
RUN addgroup -S knot \
  && adduser -S knot -G knot \
  && mkdir -p /data \
  && chown -R knot:knot /data

# Set user and working directory
USER knot
WORKDIR /data

VOLUME [ "/data" ]

EXPOSE 3000/tcp
EXPOSE 3010/tcp

# Set the entrypoint
CMD ["/usr/local/bin/knot", "server"]

LABEL org.opencontainers.image.version=v${VERSION}
LABEL org.opencontainers.image.title=Knot
LABEL org.opencontainers.image.description="Tool for creating and managing cloud-based development environments"
LABEL org.opencontainers.image.url=https://getknot.dev
LABEL org.opencontainers.image.documentation=https://getknot.dev
LABEL org.opencontainers.image.vendor="Paul Arlott"
LABEL org.opencontainers.image.licenses=Apache-2.0
LABEL org.opencontainers.image.source="https://github.com/paularlott/knot"
