## clean-testcache: clean testcache
clean-testcache:
	@echo "--> Clearing testcache"
	@go clean --testcache
.PHONY: clean-testcache

## test: Running unit tests
test:
	@echo "--> Running unit tests"
	@go test -race -covermode=atomic -coverprofile=coverage.txt ./...
.PHONY: test

## test-e2e: Running e2e tests
test-e2e: build build-da
	@echo "--> Running e2e tests"
	@go test -mod=readonly -failfast -timeout=15m -tags='e2e' ./test/e2e/... --binary=$(CURDIR)/build/testapp
.PHONY: test-e2e

## cover: generate to code coverage report.
cover:
	@echo "--> Generating Code Coverage"
	@go install github.com/ory/go-acc@latest
	@go-acc -o coverage.txt ./...
.PHONY: cover
