# Build the application binary
build:
	@echo "Building lazyazurestorage..."
	@go build -o lazyazurestorage .

# Install the application binary to $GOPATH/bin
install:
	@echo "Installing lazyazurestorage to $(go env GOPATH)/bin..."
	@go install .

.PHONY: build install
