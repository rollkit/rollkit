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

## test-all: Running all tests including Docker E2E
test-all: test test-docker-e2e-celestia
	@echo "--> All tests completed"
.PHONY: test-all

## test-integration: Running integration tests
test-integration:
	@echo "--> Running integration tests"
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

## test-docker-e2e-celestia: Running Docker E2E tests with Celestia
test-docker-e2e-celestia: docker-build-if-local
	@echo "--> Running Docker E2E tests with Celestia"
	@echo "--> Verifying Docker image exists locally..."
	@if [ -z "$(ROLLKIT_IMAGE_REPO)" ] || [ "$(ROLLKIT_IMAGE_REPO)" = "ev-node" ]; then \
		echo "--> Verifying Docker image exists locally..."; \
		docker image inspect evstack:local-dev >/dev/null 2>&1 || (echo "ERROR: evstack:local-dev image not found. Run 'make docker-build' first." && exit 1); \
	fi
	@cd test/docker-e2e && go test -mod=readonly -failfast -v -tags='docker_e2e' -timeout=30m ./...
	@$(MAKE) docker-cleanup-if-local
.PHONY: test-docker-e2e-celestia

## test-docker-compose-e2e: Running Docker Compose E2E tests against locally running stack
test-docker-compose-e2e:
	@echo "--> Running Docker Compose E2E tests"
	@echo "--> Note: This assumes docker-compose services are already running"
	@echo "--> Start services first with: cd apps/evm/single && docker compose up -d"
	@cd test/e2e && go test -v -timeout=10m -tags='evm' -run TestDockerComposeE2E ./...
.PHONY: test-docker-compose-e2e

## docker-build-if-local: Build Docker image if using local repository
docker-build-if-local:
	@if [ -z "$(ROLLKIT_IMAGE_REPO)" ] || [ "$(ROLLKIT_IMAGE_REPO)" = "evstack" ]; then \
		echo "--> Local repository detected, building Docker image..."; \
		$(MAKE) docker-build; \
	else \
		echo "--> Using remote repository: $(ROLLKIT_IMAGE_REPO)"; \
	fi
.PHONY: docker-build-if-local

## docker-cleanup-if-local: Clean up local Docker image if using local repository
docker-cleanup-if-local:
	@if [ -z "$(ROLLKIT_IMAGE_REPO)" ] || [ "$(ROLLKIT_IMAGE_REPO)" = "evstack" ]; then \
		echo "--> Untagging local Docker image (preserving layers for faster rebuilds)..."; \
		docker rmi evstack:local-dev --no-prune 2>/dev/null || echo "Image evstack:local-dev not found or already removed"; \
	else \
		echo "--> Using remote repository, no cleanup needed"; \
	fi
.PHONY: docker-cleanup-if-local
