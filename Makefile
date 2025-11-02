.PHONY: help build test test-unit test-integration coverage lint fmt vet clean install deps tidy check all goimports

# Default target
.DEFAULT_GOAL := help

# Binary name
BINARY_NAME=foundry
BUILD_DIR=bin
COVERAGE_DIR=coverage

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet
GOTOOL=$(GOCMD) tool

# Build flags
LDFLAGS=-ldflags "-s -w"

## help: Display this help message
help:
	@echo "Foundry - Libvirt VM Management Tool"
	@echo ""
	@echo "Usage:"
	@echo "  make <target>"
	@echo ""
	@echo "Targets:"
	@grep -E '^##' $(MAKEFILE_LIST) | sed 's/## /  /' | column -t -s ':'

## all: Run all checks (goimports, fmt, vet, lint, test, build)
all: goimports fmt vet lint test build

## build: Build the binary
build:
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/foundry

## install: Install the binary to $GOPATH/bin
install:
	@echo "Installing $(BINARY_NAME)..."
	$(GOCMD) install $(LDFLAGS) ./cmd/foundry

## test: Run all tests
test:
	@echo "Running tests..."
	$(GOTEST) -v -race ./...

## test-unit: Run only unit tests (exclude integration tests)
test-unit:
	@echo "Running unit tests..."
	$(GOTEST) -v -race -short ./...

## test-integration: Run only integration tests
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -race -run Integration ./...

## coverage: Run tests with coverage report
coverage:
	@echo "Running tests with coverage..."
	@mkdir -p $(COVERAGE_DIR)
	$(GOTEST) -race -coverprofile=$(COVERAGE_DIR)/coverage.out -covermode=atomic ./...
	$(GOCMD) tool cover -html=$(COVERAGE_DIR)/coverage.out -o $(COVERAGE_DIR)/coverage.html
	$(GOCMD) tool cover -func=$(COVERAGE_DIR)/coverage.out
	@echo ""
	@echo "Coverage report generated: $(COVERAGE_DIR)/coverage.html"

## lint: Run golangci-lint using go tool
lint:
	@echo "Running golangci-lint..."
	$(GOCMD) tool golangci-lint run --config .golangci.yml ./...

## goimports: Run goimports to organize imports
goimports:
	@echo "Running goimports..."
	$(GOCMD) tool goimports -w -local github.com/jbweber/foundry .

## fmt: Format all Go files
fmt:
	@echo "Formatting Go files..."
	$(GOFMT) ./...

## vet: Run go vet
vet:
	@echo "Running go vet..."
	$(GOVET) ./...

## check: Run quick checks (goimports, fmt, vet, test)
check: goimports fmt vet test

## deps: Download dependencies
deps:
	@echo "Downloading dependencies..."
	$(GOGET) -v ./...

## tidy: Tidy up go.mod and go.sum
tidy:
	@echo "Tidying go.mod and go.sum..."
	$(GOMOD) tidy

## clean: Remove build artifacts and coverage reports
clean:
	@echo "Cleaning..."
	@rm -rf $(BUILD_DIR) $(COVERAGE_DIR)
	@rm -f coverage.out coverage.html coverage.txt
	@$(GOCMD) clean

## verify: Verify dependencies and check for issues
verify:
	@echo "Verifying dependencies..."
	$(GOMOD) verify
	$(GOMOD) download
