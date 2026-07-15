.PHONY: build install test vet fmt clean version dist

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

# Cross-compile version-stamped release binaries into dist/.
DIST_PLATFORMS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64

dist:
	rm -rf dist && mkdir -p dist
	@for p in $(DIST_PLATFORMS); do \
		os=$${p%/*}; arch=$${p#*/}; ext=""; \
		[ "$$os" = "windows" ] && ext=".exe"; \
		echo "  $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			go build -trimpath $(LDFLAGS) -o dist/north_$${os}_$${arch}$$ext ./cmd/north || exit 1; \
	done

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
