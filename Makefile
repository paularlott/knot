# Set the name of your project
PROJECT_NAME := knot

# Set the platforms to build for
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Build time
BUILT_AT := $(shell date +%Y%m%d.%H%M%S%z)

# Set the flags to use for building
BUILD_FLAGS := -ldflags="-s -w -X github.com/paularlott/knot/build.Date=$(BUILT_AT)" -tags=netgo -installsuffix netgo -trimpath

# Set the output directory
OUTPUT_DIR := bin

# Get the VERSION from go run ./scripts/getversion
VERSION := $(shell go run ./scripts/getversion)

default: all

.PHONY: build
## Build the binary for the current platform
build: legal/license.txt legal/notice.txt
	go build $(BUILD_FLAGS) -o $(OUTPUT_DIR)/$(PROJECT_NAME) .

.PHONY: all
## Build the binary for all platforms
all: $(PLATFORMS)

.PHONY: $(PLATFORMS)
$(PLATFORMS): legal/license.txt legal/notice.txt apidocs webassets
	GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) go build $(BUILD_FLAGS) -o $(OUTPUT_DIR)/$(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,) .
	cd $(OUTPUT_DIR); mv $(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,) knot$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,); zip $(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,).zip knot$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,)

.PHONY: checksums
## Show the ZIP file checksums
checksums:
	shasum -a 256 $(OUTPUT_DIR)/*.zip

.PHONY: legal
## Collect the licence files from dependancies and copy them to the legal directory
legal: legal/license.txt legal/notice.txt
	rm -rf ./legal/licenses
	~/go/bin/go-licenses save --save_path ./legal/licenses/ --skip_copy_source --ignore github.com/paularlott/knot .

## Copy the application licence file fpr inclusion in the binary
legal/license.txt: LICENSE.txt
	cat LICENSE.txt > legal/license.txt

## Copy the application notice file for inclusion in the binary
legal/notice.txt: NOTICE.txt
	cat NOTICE.txt > legal/notice.txt

## Generate the API documentation
apidocs:
	npx @redocly/cli build-docs --output ./web/public_html/api-docs/index.html --config ./redocly-config.yaml --disableGoogleFont
	sed -i '' 's|<script src="https.*</script>||g' ./web/public_html/api-docs/index.html
	npx @redocly/cli build-docs --output ./web/public_html/api-docs/agent/index.html --config ./redocly-config-agent.yaml --disableGoogleFont
	sed -i '' 's|<script src="https.*</script>||g' ./web/public_html/api-docs/agent/index.html

## Compile LESS and JavaScript
webassets:
	npx vite build

.PHONY: clean
## Remove the previous build
clean:
	rm -rf $(OUTPUT_DIR)/*
	rm -rf legal/license.txt
	rm -rf legal/notice.txt
	rm -rf web/public_html/api-docs/index.html
	rm -rf web/public_html/css/*
	rm -rf web/public_html/js/*

.PHONEY: container
## Build the docker image and push to GitHub
container:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--tag paularlott/knot:$(VERSION) \
		--tag paularlott/knot:latest \
		--push \
		.

.PHONY: release
## Tag, build, and create a GitHub release
release: tag all container create-release checksums

.PHONY: tag
## Tag the current code
tag:
	git tag -a v$(VERSION) -m "Release $(VERSION)"
	git push origin v$(VERSION)

.PHONY: create-release
## Create a GitHub release
create-release:
	gh release create v$(VERSION) $(OUTPUT_DIR)/*.zip -t "Release $(VERSION)" -n "Knot $(VERSION)"

.PHONY: run-server
## Run the server for development
run-server:
	go run . server --log-level=debug --download-path=./bin

.PHONY: run-remote
## Run a remote server for development
run-remote:
	go run . server --log-level=debug --download-path=./bin --config=.knot-moon.yml

.PHONY: help
## This help screen
help:
	@printf "Available targets:\n\n"
	@awk '/^[a-zA-Z\-_0-9%:\\]+/ { \
		helpMessage = match(lastLine, /^## (.*)/); \
		if (helpMessage) { \
			helpCommand = $$1; \
			helpMessage = substr(lastLine, RSTART + 3, RLENGTH); \
			gsub("\\\\", "", helpCommand); \
			gsub(":+$$", "", helpCommand); \
			printf "  \x1b[32;01m%-20s\x1b[0m %s\n", helpCommand, helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST) | sort -u
	@printf "\n"
