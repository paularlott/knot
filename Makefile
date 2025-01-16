# Set the name of your project
PROJECT_NAME := knot

# Set the platforms to build for
PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

# Set the platforms to build the agents for
AGENT_PLATFORMS := linux/amd64 linux/arm64

# Build time
BUILT_AT := $(shell date +%Y%m%d.%H%M%S%z)

# Set the flags to use for building
BUILD_FLAGS := -ldflags="-s -w -X github.com/paularlott/knot/build.Date=$(BUILT_AT)" -trimpath

# Set the output directory
OUTPUT_DIR := bin

# Set the output directory for the agents
AGENT_OUTPUT_DIR := web/agents

# Get the VERSION from go run ./scripts/getversion
VERSION := $(shell go run ./scripts/getversion)

# Set the target registry for the container
REGISTRY ?= paularlott

default: all

.PHONY: agents build
## Build the binary for the current platform
build: agents legal/license.txt legal/notice.txt
	go build $(BUILD_FLAGS) -o $(OUTPUT_DIR)/$(PROJECT_NAME) .

.PHONY: all
## Build the binary for all platforms
all: agents $(PLATFORMS)

.PHONY: $(PLATFORMS)
$(PLATFORMS): legal/license.txt legal/notice.txt apidocs webassets
	GOOS=$(word 1,$(subst /, ,$@)) GOARCH=$(word 2,$(subst /, ,$@)) go build $(BUILD_FLAGS) -o $(OUTPUT_DIR)/$(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,) .
	cd $(OUTPUT_DIR); \
	mv $(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,) knot$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,); \
	zip $(PROJECT_NAME)_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$@))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,).zip knot$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,)

.PHONY: agents
## Build the agent binaries
agents: $(addsuffix -agent,$(AGENT_PLATFORMS))
	rm -f $(AGENT_OUTPUT_DIR)/knot-agent

.PHONY: $(addsuffix -agent,$(AGENT_PLATFORMS))
$(addsuffix -agent,$(AGENT_PLATFORMS)):
	GOOS=$(word 1,$(subst /, ,$(subst -agent,,$@))) GOARCH=$(word 2,$(subst /, ,$(subst -agent,,$@))) go build $(BUILD_FLAGS) -o $(AGENT_OUTPUT_DIR)/$(PROJECT_NAME)_agent_$(word 1,$(subst /, ,$(subst -agent,,$@)))_$(word 2,$(subst /, ,$(subst -agent,,$@)))$(if $(filter windows,$(word 1,$(subst /, ,$(subst -agent,,$@)))),.exe,) ./agent
	cd $(AGENT_OUTPUT_DIR); \
	mv $(PROJECT_NAME)_agent_$(word 1,$(subst /, ,$(subst -agent,,$@)))_$(word 2,$(subst /, ,$(subst -agent,,$@)))$(if $(filter windows,$(word 1,$(subst /, ,$(subst -agent,,$@)))),.exe,) knot-agent$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,); \
	zip $(PROJECT_NAME)_agent_$(word 1,$(subst /, ,$@))_$(word 2,$(subst /, ,$(subst -agent,,$@)))$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,).zip knot-agent$(if $(filter windows,$(word 1,$(subst /, ,$@))),.exe,)

.PHONY: checksums
## Show the ZIP file checksums
checksums:
	shasum -a 256 $(OUTPUT_DIR)/*.zip

.PHONY: homebrew-formula
## Generate the Homebrew formula
homebrew-formula:
	go run ./scripts/homebrew-formula/ > ../homebrew-tap/Formula/knot.rb

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
	sed -i.bak 's|<script src="https.*</script>||g' ./web/public_html/api-docs/index.html && rm ./web/public_html/api-docs/index.html.bak

## Run a preview server for the API documentation
apidocs-preview:
	npx @redocly/cli preview-docs api/apiv1/spec/knot-openapi.yaml

## Compile LESS/CSS and JavaScript
webassets:
	npx vite build

## Watch for changes in LESS, CSS, JavaScript files and recompile
watch:
	npx vite build --watch

.PHONY: clean
## Remove the previous build
clean:
	rm -rf $(OUTPUT_DIR)/*
	rm -rf $(AGENT_OUTPUT_DIR)/*
	rm -rf legal/license.txt
	rm -rf legal/notice.txt
	rm -rf web/public_html/api-docs/index.html
	rm -rf web/public_html/css/*
	rm -rf web/public_html/js/*

.PHONEY: container
## Build the docker image and push to Docker Hub
container:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--tag $(REGISTRY)/knot:$(VERSION) \
		--tag $(REGISTRY)/knot:latest \
		--push \
		.

.PHONY: release
## Tag, build, and create a GitHub release
release: tag all container create-release checksums homebrew-formula

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
	go run . server --log-level=debug --download-path=./bin --html-path=./web/public_html --template-path=./web/templates --agent-path=./web/agents

.PHONY: run-leaf
## Run a leaf server for development
run-leaf:
	go run . server --log-level=debug --download-path=./bin --html-path=./web/public_html --template-path=./web/templates --config=.knot-leaf1.yml

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
