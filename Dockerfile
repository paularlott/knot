ARG DOCKER_HUB

FROM ${DOCKER_HUB}library/golang:1.24.0-alpine AS builder

RUN apk update \
  && apk add bash unzip zip make nodejs npm

WORKDIR /app

COPY . ./

# Install npm dependencies
RUN npm install

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
	\
	# Download all dependencies. Dependencies will be cached if the go.mod and go.sum files are not changed
	go mod download

# Build the clients
RUN make all

# Build the application for the current architecture
RUN make build

FROM ${DOCKER_HUB}library/alpine:3.21

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

ENV KNOT_DOWNLOAD_PATH=/srv
ENV KNOT_BADGERDB_ENABLED=true
ENV KNOT_BADGERDB_PATH=/data
ENV KNOT_MEMORYDB_ENABLED=true

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
