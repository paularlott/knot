GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Builds the peer into bin/ using the universal-bundle naming knot resolves
# per host: bin/demolib_<goos>_<goarch>. Cross-compile by setting GOOS/GOARCH.
.PHONY: peer clean

peer:
	cd peer && go build -o ../bin/demolib_$(GOOS)_$(GOARCH) .

clean:
	rm -rf bin
