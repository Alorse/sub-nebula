# Sub-Nebula Makefile

.PHONY: build run test fmt lint clean

# Build the binary
build:
	go build -o sub-nebula .

# Run the server
run:
	go run .

# Run tests
test:
	go test -v ./...

# Run tests in short mode (skip integration tests)
test-short:
	go test -v -short ./...

# Format all Go files
fmt:
	gofumpt -w .
	goimports -w .

# Check formatting without modifying files
fmt-check:
	@test -z "$(shell gofumpt -l .)" || (echo "Files need formatting:" && gofumpt -l . && exit 1)

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -f sub-nebula

# Install development tools
install-tools:
	go install mvdan.cc/gofumpt@latest
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Full check (format, test, lint)
check: fmt-check test lint
