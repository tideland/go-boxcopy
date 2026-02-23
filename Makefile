.PHONY: all help tidy lint build binary cross test bench coverage clean install-tools check-tools ci install

# Default target
all: tidy lint binary test

# Help target
help:
	@echo "Available targets:"
	@echo "  all           - Run tidy, lint, build, and test (default)"
	@echo "  help          - Show this help message"
	@echo "  tidy          - Update go.mod and go.sum"
	@echo "  lint          - Run golangci-lint"
	@echo "  build         - Build the package"
	@echo "  binary        - Build boxcopy binary in current directory"
	@echo "  test          - Run tests with race detector"
	@echo "  bench         - Run benchmarks"
	@echo "  coverage      - Generate coverage report"
	@echo "  clean         - Remove build artifacts and cache"
	@echo "  install-tools - Install development tools"
	@echo "  check-tools   - Check tool versions"
	@echo "  ci            - Run CI pipeline (tidy, lint, build, test)"
	@echo "  cross         - Cross-compile boxcopy for Linux, macOS, and Windows (amd64/arm64)"
	@echo "  install       - Install boxcopy binary"

# Tidy go.mod and go.sum
tidy:
	@echo "==> Tidying go.mod and go.sum..."
	go mod tidy

# Run golangci-lint
lint:
	@echo "==> Running golangci-lint..."
	golangci-lint run --timeout 5m

# Build the package
build:
	@echo "==> Building package..."
	go build -v ./...

# Build the boxcopy binary
binary:
	@echo "==> Building boxcopy binary..."
	go build -v -o boxcopy ./cmd/boxcopy

# Cross-compile for Linux, macOS, and Windows (amd64 and arm64)
CROSS_TARGETS = \
	linux/amd64 \
	linux/arm64 \
	darwin/amd64 \
	darwin/arm64 \
	windows/amd64 \
	windows/arm64

cross:
	@echo "==> Cross-compiling boxcopy..."
	@mkdir -p dist
	@for target in $(CROSS_TARGETS); do \
		os=$$(echo $$target | cut -d/ -f1); \
		arch=$$(echo $$target | cut -d/ -f2); \
		out="dist/boxcopy-$$os-$$arch"; \
		[ "$$os" = "windows" ] && out="$$out.exe"; \
		echo "  Building $$os/$$arch -> $$out"; \
		GOOS=$$os GOARCH=$$arch go build -o "$$out" ./cmd/boxcopy || exit 1; \
	done
	@echo "==> Binaries written to dist/"

# Run tests with race detector
test:
	@echo "==> Running tests..."
	go test -v -race ./...

# Run benchmarks
bench:
	@echo "==> Running benchmarks..."
	go test -bench=. -benchmem ./...

# Generate coverage report
coverage:
	@echo "==> Generating coverage report..."
	go test -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

# Clean build artifacts and cache
clean:
	@echo "==> Cleaning..."
	rm -f boxcopy coverage.out coverage.html
	rm -rf dist/
	go clean -cache -testcache

# Install development tools
install-tools:
	@echo "==> Installing development tools..."
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

# Check tool versions
check-tools:
	@echo "==> Checking tool versions..."
	@echo "Go version:"
	@go version
	@echo "\ngolangci-lint version:"
	@golangci-lint --version

# CI pipeline
ci: tidy lint binary test
	@echo "==> CI pipeline completed successfully"

# Install boxcopy binary
install:
	@echo "==> Installing boxcopy..."
	go install ./cmd/boxcopy
