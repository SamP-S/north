.PHONY: build install test vet fmt clean

# Build the north binary into the repo root.
build:
	go build -o north ./cmd/north

# Install north onto your PATH ($GOBIN / $GOPATH/bin).
install:
	go install ./cmd/north

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
	rm -f north
