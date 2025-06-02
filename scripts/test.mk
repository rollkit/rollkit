## clean-testcache: clean testcache
clean-testcache:
	@echo "--> Clearing testcache"
	@go clean --testcache
.PHONY: clean-testcache

## test: Running unit tests for all go.mods
test:
	@echo "--> Running unit tests"
	@go run -tags=run scripts/test.go
.PHONY: test

## test-e2e: Running e2e tests
test-e2e: build build-da build-evm-single
	@echo "--> Running e2e tests"
	@cd test/e2e && go test -mod=readonly -failfast -timeout=15m -tags='e2e evm' ./... --binary=$(CURDIR)/build/testapp --evm-binary=$(CURDIR)/build/evm-single
.PHONY: test-e2e

## cover: generate to code coverage report.
cover:
	@echo "--> Generating Code Coverage"
	@go install github.com/ory/go-acc@latest
	@go-acc -o coverage.txt ./...
.PHONY: cover

## test-cover: generate code coverage report.
test-cover:
	@echo "--> Running unit tests"
	@go run -tags=cover scripts/test_cover.go
.PHONY: test-cover

## test-evm: Running EVM tests
test-evm: build-evm-single
	@echo "--> Running EVM tests"
	@cd execution/evm && go test -mod=readonly -failfast -timeout=15m ./... -tags=evm
