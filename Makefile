.PHONY: all build build-all clean test fmt vet install help

# Build variables
BINARY_NAME=etcd-secret-reader
VERSION?=dev
BUILD_DIR=build
CMD_DIR=./cmd/etcd-secret-reader
LDFLAGS=-ldflags "-s -w -X main.version=$(VERSION)"

# Go build flags
GOFLAGS=-trimpath
CGO_ENABLED=0

# Default target
all: build

# Build for current platform
build:
	@echo "Building $(BINARY_NAME)..."
	go build $(GOFLAGS) $(LDFLAGS) -o $(BINARY_NAME) $(CMD_DIR)
	@echo "Build complete: $(BINARY_NAME)"

# Build for all platforms
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p $(BUILD_DIR)

	# Linux AMD64
	@echo "Building for Linux AMD64..."
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(CMD_DIR)

	# Linux ARM64
	@echo "Building for Linux ARM64..."
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(CMD_DIR)

	# macOS AMD64
	@echo "Building for macOS AMD64..."
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(CMD_DIR)

	# macOS ARM64 (Apple Silicon)
	@echo "Building for macOS ARM64..."
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(CMD_DIR)

	# Windows AMD64
	@echo "Building for Windows AMD64..."
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) \
		-o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(CMD_DIR)

	@echo "Build complete! Binaries are in $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/

# Run tests
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Run go vet
vet:
	@echo "Running go vet..."
	go vet ./...

# Check code formatting
check-fmt:
	@echo "Checking code formatting..."
	@if [ -n "$$(gofmt -s -l .)" ]; then \
		echo "Code is not formatted. Run 'make fmt' to format."; \
		gofmt -s -d .; \
		exit 1; \
	fi
	@echo "Code is formatted correctly."

# Download dependencies
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod verify

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	go mod tidy

# Install binary to /usr/local/bin
install: build
	@echo "Installing $(BINARY_NAME) to /usr/local/bin..."
	@sudo mv $(BINARY_NAME) /usr/local/bin/
	@echo "Installation complete!"

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)
	@rm -f coverage.txt
	@echo "Clean complete!"

# Show version
version:
	@echo "Version: $(VERSION)"
	@if [ -f $(BINARY_NAME) ]; then \
		./$(BINARY_NAME) --version; \
	else \
		echo "Binary not built yet. Run 'make build' first."; \
	fi

# Create release archives (like GitHub Actions does)
release: build-all
	@echo "Creating release archives..."
	@mkdir -p $(BUILD_DIR)/archives

	# Linux AMD64
	tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-linux-amd64.tar.gz \
		-C $(BUILD_DIR) $(BINARY_NAME)-linux-amd64 \
		-C .. README.md examples/

	# Linux ARM64
	tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-linux-arm64.tar.gz \
		-C $(BUILD_DIR) $(BINARY_NAME)-linux-arm64 \
		-C .. README.md examples/

	# macOS AMD64
	tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-darwin-amd64.tar.gz \
		-C $(BUILD_DIR) $(BINARY_NAME)-darwin-amd64 \
		-C .. README.md examples/

	# macOS ARM64
	tar -czf $(BUILD_DIR)/archives/$(BINARY_NAME)-$(VERSION)-darwin-arm64.tar.gz \
		-C $(BUILD_DIR) $(BINARY_NAME)-darwin-arm64 \
		-C .. README.md examples/

	# Windows AMD64
	cd $(BUILD_DIR) && zip archives/$(BINARY_NAME)-$(VERSION)-windows-amd64.zip \
		$(BINARY_NAME)-windows-amd64.exe ../README.md ../examples/*

	# Generate checksums
	cd $(BUILD_DIR)/archives && sha256sum * > checksums.txt

	@echo "Release archives created in $(BUILD_DIR)/archives/"
	@ls -lh $(BUILD_DIR)/archives/

# Run all checks (formatting, vetting, tests)
check: check-fmt vet test
	@echo "All checks passed!"

# Help target
help:
	@echo "Available targets:"
	@echo "  make build       - Build for current platform"
	@echo "  make build-all   - Build for all platforms"
	@echo "  make test        - Run tests"
	@echo "  make fmt         - Format code"
	@echo "  make vet         - Run go vet"
	@echo "  make check-fmt   - Check code formatting"
	@echo "  make check       - Run all checks (fmt, vet, test)"
	@echo "  make deps        - Download dependencies"
	@echo "  make tidy        - Tidy dependencies"
	@echo "  make install     - Install binary to /usr/local/bin"
	@echo "  make clean       - Clean build artifacts"
	@echo "  make version     - Show version"
	@echo "  make release     - Create release archives (set VERSION=v1.0.0)"
	@echo "  make help        - Show this help message"
	@echo ""
	@echo "Examples:"
	@echo "  make build                    - Build with version 'dev'"
	@echo "  make build VERSION=v1.0.0     - Build with version 'v1.0.0'"
	@echo "  make release VERSION=v1.0.0   - Create release archives"
