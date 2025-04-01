DOCKER := $(shell which docker)
DOCKER_BUF := $(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace bufbuild/buf
PACKAGE_NAME          := github.com/rollkit/rollkit
GOLANG_CROSS_VERSION  ?= v1.22.1

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
	@sed -n 's/^##//p' $< | column -t -s ':' | sort | sed -e 's/^/ /'
.PHONY: help

## clean-testcache: clean testcache
clean-testcache:
	@echo "--> Clearing testcache"
	@go clean --testcache
.PHONY: clean-testcache

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
	@go mod tidy
	@go run scripts/go-mod-tidy-all.go
.PHONY: deps

tidy-all:
	@go run scripts/go-mod-tidy-all.go
.PHONY: tidy-all

## lint: Run linters golangci-lint and markdownlint.
lint: vet
	@echo "--> Running golangci-lint"
	@golangci-lint run
	@echo "--> Running markdownlint"
	@markdownlint --config .markdownlint.yaml --ignore './specs/src/specs/**.md' --ignore './cmd/rollkit/docs/*.md' '**/*.md'
	@echo "--> Running hadolint"
	@hadolint test/docker/mockserv.Dockerfile
	@echo "--> Running yamllint"
	@yamllint --no-warnings . -c .yamllint.yml
	@echo "--> Running goreleaser check"
	@goreleaser check
	@echo "--> Running actionlint"
	@actionlint

.PHONY: lint

## fmt: Run fixes for linters.
lint-fix:
	@echo "--> Formatting go"
	@golangci-lint run --fix
	@echo "--> Formatting markdownlint"
	@markdownlint --config .markdownlint.yaml --ignore './specs/src/specs/**.md' --ignore './cmd/rollkit/docs/*.md' '**/*.md' -f
.PHONY: lint-fix

## vet: Run go vet
vet: 
	@echo "--> Running go vet"
	@go vet $(pkgs)
.PHONY: vet

## test: Running unit tests
test: vet
	@echo "--> Running unit tests"
	@go test -race -covermode=atomic -coverprofile=coverage.txt $(pkgs) -run $(run) -count=$(count)
.PHONY: test

## test-e2e: Running e2e tests
test-e2e: build
	@echo "--> Running e2e tests"
	@go test -mod=readonly -failfast -timeout=15m -tags='e2e' ./test/e2e/... --binary=$(CURDIR)/build/rollkit
.PHONY: test-e2e

## proto-gen: Generate protobuf files. Requires docker.
proto-gen:
	@echo "--> Generating Protobuf files"
	buf generate --path="./proto/rollkit" --template="buf.gen.yaml" --config="buf.yaml"
.PHONY: proto-gen

## mock-gen: generate mocks of external (commetbft) types
mock-gen:
	@echo "-> Generating mocks"
	mockery --output da/mocks --srcpkg github.com/rollkit/rollkit/core/da --name DA
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/store --name Store
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
	@echo "--> Installing Testapp CLI"
	@go install -ldflags "$(LDFLAGS)" ./rollups/testapp
	@echo "--> Testapp CLI Installed!"
	@echo "    Check the version with: testapp version"
	@echo "    Check the binary with: which testapp"
.PHONY: install

## build: build rollkit CLI
build:
	@echo "--> Building Testapp CLI"
	@mkdir -p $(CURDIR)/build
	@go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/testapp ./rollups/testapp
	@echo "--> Testapp CLI Built!"
	@echo "    Check the version with: rollups/testapp version"
	@echo "    Check the binary with: $(CURDIR)/rollups/testapp"
.PHONY: build

build-da:
	@echo "--> Building local-da"
	@mkdir -p $(CURDIR)/build
	@cd da && go build -ldflags "$(LDFLAGS)" -o $(CURDIR)/build/local-da ./cmd/local-da
.PHONY: build-da

## clean: clean and build
clean: 
	@echo "--> Cleaning Testapp CLI"
	@rm -rf $(CURDIR)/build/testapp
	@echo "--> Testapp CLI Cleaned!"
.PHONY: clean

## prebuilt-binary: Create prebuilt binaries and attach them to GitHub release. Requires Docker.
prebuilt-binary:
	@if [ ! -f ".release-env" ]; then \
		echo "A .release-env file was not found but is required to create prebuilt binaries. This command is expected to be run in CI where a .release-env file exists. If you need to run this command locally to attach binaries to a release, you need to create a .release-env file with a Github token (classic) that has repo:public_repo scope."; \
		exit 1;\
	fi
	docker run \
		--rm \
		--env-file .release-env \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(PACKAGE_NAME) \
		-w /go/src/$(PACKAGE_NAME) \
		ghcr.io/goreleaser/goreleaser-cross:${GOLANG_CROSS_VERSION} \
		release --clean
.PHONY: prebuilt-binary
