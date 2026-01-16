.PHONY: build install test check lint fmt vet clean

# Build all binaries to current directory
build:
	go build -o clawde ./cmd/clawde
	go build -o clawde-diff ./cmd/clawde-diff

# Install all binaries to $GOPATH/bin
install:
	go install ./cmd/...

# Run all tests
test:
	go test ./...

# Run linting then tests
check: lint test

# Run all linters
lint: fmt vet

# Format code
fmt:
	@test -z "$$(gofmt -l .)" || (gofmt -d . && exit 1)

# Run go vet
vet:
	go vet ./...

# Format code in place
fmt-fix:
	gofmt -w .

# Tidy go modules
tidy:
	go mod tidy

# Clean build artifacts
clean:
	rm -f clawde clawde-diff
	go clean ./...
