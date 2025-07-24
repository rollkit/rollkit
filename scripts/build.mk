# Extract the latest Git tag as the version number
VERSION := $(shell git describe --tags --abbrev=0)
GITSHA := $(shell git rev-parse --short HEAD)
LDFLAGS := \
	-X github.com/evstack/ev-node/pkg/cmd.Version=$(VERSION) \
	-X github.com/evstack/ev-node/pkg/cmd.GitSHA=$(GITSHA)

## build: build Testapp CLI
build:
	@echo "--> Building Testapp CLI"
	@mkdir -p $(CURDIR)/build
	@cd apps/testapp && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/testapp .
	@echo "--> Testapp CLI Built!"
	@echo "    Check the version with: build/testapp version"
        @echo "    Check the binary with: $(CURDIR)/build/testapp"
.PHONY: build

## install: Install Testapp CLI
install:
	@echo "--> Installing Testapp CLI"
	@cd apps/testapp && go install -ldflags "$(LDFLAGS)" .
	@echo "--> Testapp CLI Installed!"
	@echo "    Check the version with: testapp version"
	@echo "    Check the binary with: which testapp"
.PHONY: install

build-all:
	@echo "--> Building all rollkit binaries"
	@mkdir -p $(CURDIR)/build
	@echo "--> Building testapp"
	@cd apps/testapp && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/testapp .
	@echo "--> Building evm-single"
	@cd apps/evm/single && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/evm-single .
	@echo "--> Building evm-based"
	@cd apps/evm/based && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/evm-based .
	@echo "--> Building local-da"
	@cd da && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/local-da ./cmd/local-da
	@echo "--> All rollkit binaries built!"

## build-testapp-bench:
build-testapp-bench:
	@echo "--> Building Testapp Bench"
	@mkdir -p $(CURDIR)/build
	@cd apps/testapp && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/testapp-bench ./kv/bench
	@echo "    Check the binary with: $(CURDIR)/build/testapp-bench"
.PHONY: build-testapp-bench

## build-evm-single: build evm single
build-evm-single:
	@echo "--> Building EVM single"
	@mkdir -p $(CURDIR)/build
	@cd apps/evm/single && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/evm-single .
        @echo "    Check the binary with: $(CURDIR)/build/evm-single"

## build-evm-based: build evm based
build-evm-based:
	@echo "--> Building EVM based"
	@mkdir -p $(CURDIR)/build
	@cd apps/evm/based && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/evm-based .
        @echo "    Check the binary with: $(CURDIR)/build/evm-based"

build-da:
	@echo "--> Building local-da"
	@mkdir -p $(CURDIR)/build
	@cd da && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/local-da ./cmd/local-da
        @echo "    Check the binary with: $(CURDIR)/build/local-da"
.PHONY: build-da

## docker-build: Build Docker image for local testing
docker-build:
	@echo "--> Building Docker image for local testing"
	@docker build -t rollkit:local-dev .
	@echo "--> Docker image built: rollkit:local-dev"
	@echo "--> Checking if image exists locally..."
	@docker images rollkit:local-dev
.PHONY: docker-build

## clean: clean and build
clean:
	@echo "--> Cleaning Testapp CLI"
	@rm -rf $(CURDIR)/build/
	@echo "--> Testapp CLI Cleaned!"
.PHONY: clean
