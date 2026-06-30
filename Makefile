.PHONY: build install test vet fmt clean version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS  := -ldflags "-X github.com/SamP-S/north/internal/version.Version=$(VERSION)"

# Build the north binary into bin/.
build:
	go build $(LDFLAGS) -o bin/north ./cmd/north

# Install north onto your PATH ($GOBIN / $GOPATH/bin).
install:
	go install $(LDFLAGS) ./cmd/north

# Print the version that would be stamped into the binary.
version:
	@echo $(VERSION)

# Run the test suite.
test:
	go test ./...

# Static checks: go vet plus a gofmt formatting check.
vet:
	go vet ./...
	@unformatted=$$(gofmt -l cmd internal); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt needed on:"; echo "$$unformatted"; exit 1; \
	fi

# Format all Go sources in place.
fmt:
	gofmt -w cmd internal

clean:
	rm -f bin/north
