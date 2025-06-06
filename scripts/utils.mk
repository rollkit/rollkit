## deps: Install dependencies
deps:
	@echo "--> Installing dependencies"
	@go mod download
	@go mod tidy
	@go run scripts/go-mod-tidy-all.go
.PHONY: deps

tidy-all:
	@go run -tags=tidy scripts/tidy.go
.PHONY: tidy-all

## lint: Run linters golangci-lint and markdownlint.
lint: vet
	@echo "--> Running golangci-lint"
	@golangci-lint run
	@echo "--> Running markdownlint"
	@markdownlint --config .markdownlint.yaml --ignore './specs/src/specs/**.md' '**/*.md'
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
	@markdownlint --config .markdownlint.yaml --ignore './specs/src/specs/**.md' '**/*.md' -f
.PHONY: lint-fix

## vet: Run go vet
vet: 
	@echo "--> Running go vet"
	@go vet ./...
.PHONY: vet

## mock-gen: generate mocks of external (commetbft) types
mock-gen:
	@echo "-> Generating mocks"
	mockery --output da/internal/mocks --srcpkg github.com/rollkit/rollkit/core/da --name DA --filename="da.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/da --name DA --filename="da.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/da --name Client --filename="daclient.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/execution --name Executor --filename="execution.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/sequencer --name Sequencer --filename="sequencer.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/store --name Store --filename="store.go"
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/p2p --name P2PRPC --filename="p2p.go"
.PHONY: mock-gen

mock-header:
	mockery --output test/mocks --srcpkg github.com/celestiaorg/go-header --name Store --filename="external/hstore.go"
.PHONY: mock-header

## mockery-docker: generate mocks using Docker
mockery-docker:
	@echo "-> Generating mocks using Docker"
	docker run --rm \
		-v $(CURDIR):/src \
		-v $(GOPATH)/pkg/mod:/go/pkg/mod \
		-w /src \
		-e GOPATH=/go \
		golang:latest bash -c "\
			go mod download && \
			go mod tidy && \
			go install github.com/vektra/mockery/v2@v2.53.0 && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/da --name DA --filename da.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/da --name Client --filename daclient.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/execution --name Executor --filename execution.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/core/sequencer --name Sequencer --filename sequencer.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/store --name Store --filename store.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/p2p --name P2PRPC --filename p2p.go --with-expecter && \
			mockery --output test/mocks --srcpkg github.com/celestiaorg/go-header --name Store --filename external/hstore.go --with-expecter"
.PHONY: mockery-docker
