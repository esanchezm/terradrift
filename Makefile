.PHONY: build test lint clean

BINARY_NAME=terradrift
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%dT%H:%M:%SZ')
LDFLAGS=-ldflags "-X main.version=${VERSION} -X main.buildTime=${BUILD_TIME}"

# Build the binary
build:
	go build ${LDFLAGS} -o ${BINARY_NAME} .

# Run tests
test:
	go test -v -race ./...

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -f ${BINARY_NAME}
	rm -f terradrift_*_darwin_amd64
	rm -f terradrift_*_darwin_arm64
	rm -f terradrift_*_linux_amd64
	rm -f terradrift_*_linux_arm64
	rm -f dist/
