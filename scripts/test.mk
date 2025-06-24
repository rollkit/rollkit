## clean-testcache: clean testcache
clean-testcache:
	@echo "--> Clearing testcache"
	@go clean --testcache
.PHONY: clean-testcache

## test: Running unit tests for all go.mods
test:
	@echo "--> Running unit tests"
	@go run -tags='run integration' scripts/test.go
.PHONY: test

## test-e2e: Running e2e tests
test-integration:
	@echo "--> Running e2e tests"
	@cd node && go test -mod=readonly -failfast -timeout=15m -tags='integration' ./...
.PHONY: test-integration

## test-e2e: Running e2e tests
test-e2e: build build-da build-evm-single
	@echo "--> Running e2e tests"
	@cd test/e2e && go test -mod=readonly -failfast -timeout=15m -tags='e2e evm' ./... --binary=$(CURDIR)/build/testapp --evm-binary=$(CURDIR)/build/evm-single
.PHONY: test-e2e

## test-integration-cover: generate code coverage report for integration tests.
test-integration-cover:
	@echo "--> Running integration tests with coverage"
	@cd node && go test -mod=readonly -failfast -timeout=15m -tags='integration' -coverprofile=coverage.txt -covermode=atomic ./...
.PHONY: test-integration-cover

## test-cover: generate code coverage report.
test-cover:
	@echo "--> Running unit tests"
	@go run -tags=cover scripts/test_cover.go
.PHONY: test-cover

## test-evm: Running EVM tests
test-evm:
	@echo "--> Running EVM tests"
	@cd execution/evm && go test -mod=readonly -failfast -timeout=15m ./... -tags=evm

## test-docker-e2e: Running Docker E2E tests
test-docker-e2e:
	@echo "--> Running Docker E2E tests"
	@cd test/docker-e2e && go test -mod=readonly -failfast -tags='docker_e2e' -timeout=30m ./...
