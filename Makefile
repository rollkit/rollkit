DOCKER := $(shell which docker)
DOCKER_BUF := $(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace bufbuild/buf

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
	@go-acc -o coverage.txt `go list ./...`
.PHONY: cover

## lint: Run linters golangci-lint and markdownlint.
lint:
	@echo "--> Running golangci-lint"
	@golangci-lint run
	@echo "--> Running markdownlint"
	@markdownlint --config .markdownlint.yaml '**/*.md'
	@echo "--> Running hadolint"
	@hadolint docker/mockserv.Dockerfile
	@echo "--> Running yamllint"
	@yamllint --no-warnings . -c .yamllint.yml

.PHONY: lint

## fmt: Run fixes for linters. Currently only markdownlint.
fmt:
	@echo "--> Formatting markdownlint"
	@markdownlint --config .markdownlint.yaml '**/*.md' -f
.PHONY: fmt

## test-unit: Running unit tests
test-unit:
	@echo "--> Running unit tests"
	@go test -covermode=atomic -coverprofile=coverage.txt `go list ./...`
.PHONY: test-unit

## test-unit-race: Running unit tests with data race detector
test-unit-race:
	@echo "--> Running unit tests with data race detector"
	@go test -race -count=1 `go list ./...`
.PHONY: test-unit-race

## test-all: Run tests with and without data race
test-all:
	@$(MAKE) test-unit
	@$(MAKE) test-unit-race
.PHONY: test-all

## proto-gen: Generate protobuf files. Requires docker.
proto-gen:
	@echo "--> Generating Protobuf files"
	./proto/get_deps.sh
	./proto/gen.sh
.PHONY: proto-gen

## proto-lint: Lint protobuf files. Requires docker.
proto-lint:
	@echo "--> Linting Protobuf files"
	@$(DOCKER_BUF) lint --error-format=json
.PHONY: proto-lint
