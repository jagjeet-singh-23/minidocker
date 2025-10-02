# Makefile for a simple Go project
# Usage: make build BINARY=myapp
# Default binary name (can override): BINARY=app
BINARY ?= app
PKG ?= ./...
BIN_DIR ?= ./bin

GO := go

.PHONY: all build run test fmt vet lint clean install deps docker-build

all: build

# build the binary to bin/
build:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/$(BINARY) .

# build with race detector (dev)
build-race:
	@mkdir -p $(BIN_DIR)
	$(GO) build -race -o $(BIN_DIR)/$(BINARY) .

# run the binary (use BINARY if you want the built one)
run: build
	$(BIN_DIR)/$(BINARY)

test:
	$(GO) test $(PKG)

test-verbose:
	$(GO) test -v $(PKG)

fmt:
	$(GO) fmt $(PKG)

vet:
	$(GO) vet $(PKG)

lint:
	@if ! command -v golangci-lint >/dev/null 2>&1; then \
	  echo "golangci-lint not found; install from https://golangci-lint.run/"; \
	  exit 0; \
	fi
	golangci-lint run

install:
	$(GO) install ./...

deps:
	$(GO) mod download

clean:
	@rm -rf $(BIN_DIR)
	@echo "cleaned $(BIN_DIR)"

# Build a docker image (optional)
# Usage: make docker-build IMAGE=yourname/app:tag
docker-build:
ifndef IMAGE
	$(error IMAGE is not set. Usage: make docker-build IMAGE=yourname/app:tag)
endif
	docker build -t $(IMAGE) .


