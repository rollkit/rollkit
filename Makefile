DOCKER := $(shell which docker)
DOCKER_BUF := $(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace bufbuild/buf

# Define pkgs, run, and cover variables for test so that we can override them in
# the terminal more easily.

# IGNORE_DIRS is a list of directories to ignore when running tests and linters.
# This list is space separated.
IGNORE_DIRS ?= third_party
pkgs := $(shell go list ./... | grep -vE "$(IGNORE_DIRS)")
run := .
count := 1

## help: Show this help message
help: Makefile
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
.PHONY: help

## clean: clean testcache
clean:
	@echo "--> Clearing testcache"
	@go clean --testcache
.PHONY: clean

## cover: generate to code coverage report.
cover:
	@echo "--> Generating Code Coverage"
	@go install github.com/ory/go-acc@latest
	@go-acc -o coverage.txt $(pkgs)
.PHONY: cover

## deps: Install dependencies
deps:
	@echo "--> Installing dependencies"
	@go mod download
	@make proto-gen
	@go mod tidy
.PHONY: deps

## lint: Run linters golangci-lint and markdownlint.
lint: vet
	@echo "--> Running golangci-lint"
	@golangci-lint run
	@echo "--> Running markdownlint"
	@markdownlint --config .markdownlint.yaml --ignore './cmd/rollkit/docs/*.md' '**/*.md'
	@echo "--> Running hadolint"
	@hadolint test/docker/mockserv.Dockerfile
	@echo "--> Running yamllint"
	@yamllint --no-warnings . -c .yamllint.yml

.PHONY: lint

## fmt: Run fixes for linters.
fmt:
	@echo "--> Formatting markdownlint"
	@markdownlint --config .markdownlint.yaml --ignore './cmd/rollkit/docs/*.md' '**/*.md' -f
	@echo "--> Formatting go"
	@golangci-lint run --fix
.PHONY: fmt

## vet: Run go vet
vet: 
	@echo "--> Running go vet"
	@go vet $(pkgs)
.PHONY: vet

## test: Running unit tests
test: vet
	@echo "--> Running unit tests"
	@go test -v -race -covermode=atomic -coverprofile=coverage.txt $(pkgs) -run $(run) -count=$(count)
.PHONY: test

## proto-gen: Generate protobuf files. Requires docker.
proto-gen:
	@echo "--> Generating Protobuf files"
	./proto/get_deps.sh
	./proto/gen.sh
.PHONY: proto-gen

## mock-gen: generate mocks of external (commetbft) types
mock-gen:
	@echo "-> Generating mocks"
	mockery --output test/mocks --srcpkg github.com/cometbft/cometbft/rpc/client --name Client
	mockery --output test/mocks --srcpkg github.com/cometbft/cometbft/abci/types --name Application
	mockery --output test/mocks --srcpkg github.com/rollkit/go-da --name DA
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/store --name Store
.PHONY: mock-gen


## proto-lint: Lint protobuf files. Requires docker.
proto-lint:
	@echo "--> Linting Protobuf files"
	@$(DOCKER_BUF) lint --error-format=json
.PHONY: proto-lint

# Extract the latest Git tag as the version number
VERSION := $(shell git describe --tags --abbrev=0)
GITSHA := $(shell git rev-parse --short HEAD)
LDFLAGS := \
	-X github.com/rollkit/rollkit/cmd/rollkit/commands.Version=$(VERSION) \
	-X github.com/rollkit/rollkit/cmd/rollkit/commands.GitSHA=$(GITSHA)

## install: Install rollkit CLI
install:
	@echo "--> Installing Rollkit CLI"
	@go install -ldflags "$(LDFLAGS)" ./cmd/rollkit
	@echo "--> Rollkit CLI Installed!"
	@echo "    Check the version with: rollkit version"
	@echo "    Check the binary with: which rollkit"
.PHONY: install
