# Clear test cache
[group('test')]
clean-testcache:
    @echo "--> Clearing testcache"
    @go clean --testcache

# Run unit tests for all go.mods
[group('test')]
test:
    @echo "--> Running unit tests"
    @go run -tags='run integration' scripts/test.go

# Run all tests including Docker E2E
[group('test')]
test-all: test test-docker-e2e
    @echo "--> All tests completed"

# Run integration tests
[group('test')]
test-integration:
    @echo "--> Running integration tests"
    @cd node && go test -mod=readonly -failfast -timeout=15m -tags='integration' ./...

# Run e2e tests
[group('test')]
test-e2e: build build-da build-evm docker-build-if-local
    @echo "--> Running e2e tests"
    @cd test/e2e && go test -mod=readonly -failfast -timeout=15m -tags='e2e evm' $(go list -tags='e2e evm' ./... | grep -v /benchmark) --binary=../../build/testapp --evm-binary=../../build/evm

# Run integration tests with coverage
[group('test')]
test-integration-cover:
    @echo "--> Running integration tests with coverage"
    @cd node && go test -mod=readonly -failfast -timeout=15m -tags='integration' -coverprofile=coverage.txt -covermode=atomic ./...

# Generate code coverage report
[group('test')]
test-cover:
    @echo "--> Running unit tests"
    @go run -tags=cover -race scripts/test_cover.go

# Run micro-benchmarks for internal cache
[group('test')]
bench:
    @echo "--> Running internal cache benchmarks"
    @go test -bench=. -benchmem -run='^$' ./block/internal/cache

# Run EVM tests
[group('test')]
test-evm:
    @echo "--> Running EVM tests"
    @cd execution/evm/test && go test -mod=readonly -failfast -timeout=15m ./... -tags=evm

# Run Docker E2E tests
[group('test')]
test-docker-e2e: docker-build-if-local
    @echo "--> Running Docker E2E tests"
    @echo "--> Verifying Docker image exists locally..."
    @if [ -z "${EV_NODE_IMAGE_REPO:-}" ] || [ "${EV_NODE_IMAGE_REPO:-}" = "ev-node" ]; then \
        echo "--> Verifying Docker image exists locally..."; \
        docker image inspect evstack:local-dev >/dev/null 2>&1 || (echo "ERROR: evstack:local-dev image not found. Run 'just docker-build' first." && exit 1); \
    fi
    @cd test/docker-e2e && go test -mod=readonly -failfast -v -tags='docker_e2e' -timeout=30m ./...
    @just docker-cleanup-if-local

# Run Docker E2E Upgrade tests
[group('test')]
test-docker-upgrade-e2e:
    @echo "--> Running Docker Upgrade E2E tests"
    @cd test/docker-e2e && go test -mod=readonly -failfast -v -tags='docker_e2e evm' -timeout=30m -run '^TestEVMSingleUpgradeSuite$$/^TestEVMSingleUpgrade$$' ./...

# Run Docker E2E signed blob submission test against the evm binary and reth
[group('test')]
test-docker-signed-submit-evm:
    @echo "--> Running Docker signed blob submission EVM E2E test"
    @cd test/docker-e2e && go test -mod=readonly -failfast -v -tags='docker_e2e evm' -timeout=30m -run '^TestDockerSuite$$/^TestSignedBlobSubmissionEVM$$' ./...

# Run Docker E2E cross-version compatibility tests
[group('test')]
test-docker-compat:
    @echo "--> Running Docker Sync Compatibility E2E tests"
    @cd test/docker-e2e && go test -mod=readonly -failfast -v -tags='docker_e2e evm' -timeout=30m -run '^TestEVMCompatSuite$$/^TestCrossVersionSync$$' ./...

# Build Docker image if using local repository
[group('test')]
docker-build-if-local:
    @if [ -z "${EV_NODE_IMAGE_REPO:-}" ] || [ "${EV_NODE_IMAGE_REPO:-}" = "evstack" ]; then \
        if docker image inspect evstack:local-dev >/dev/null 2>&1; then \
            echo "--> Found local Docker image: evstack:local-dev (skipping build)"; \
        else \
            echo "--> Local repository detected, building Docker image..."; \
            just docker-build; \
        fi; \
    else \
        echo "--> Using remote repository: ${EV_NODE_IMAGE_REPO}"; \
    fi

# Clean up local Docker image if using local repository
[group('test')]
docker-cleanup-if-local:
    @if [ -z "${EV_NODE_IMAGE_REPO:-}" ] || [ "${EV_NODE_IMAGE_REPO:-}" = "evstack" ]; then \
        echo "--> Untagging local Docker image (preserving layers for faster rebuilds)..."; \
        docker rmi evstack:local-dev --no-prune 2>/dev/null || echo "Image evstack:local-dev not found or already removed"; \
    else \
        echo "--> Using remote repository, no cleanup needed"; \
    fi
