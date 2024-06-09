FROM ${DOCKER_HUB}library/alpine:3.20.0

# Upgrade to the latest versions
RUN apk update \
  && apk upgrade \
  && apk add bash unzip

# Copy files in
COPY bin/*.zip /srv/

# Unpack appropriate zip file to /usr/local/bin
RUN ARCH=$(uname -m); \
  case "$ARCH" in \
		'x86_64') f="knot_linux_amd64";; \
		'aarch64') f="knot_linux_arm64";; \
		*) echo >&2 "error: unsupported architecture: '$ARCH'"; exit 1 ;; \
	esac \
  && unzip /srv/$f.zip -d /usr/local/bin \
  \
  # Add a user, knot, to run the process
  && addgroup -S knot \
  && adduser -S knot -G knot

# Set user and working directory
USER knot
WORKDIR /home/knot

# Set the entrypoint
CMD ["/usr/local/bin/knot", "server"]
