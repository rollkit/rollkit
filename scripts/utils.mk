## deps: Install dependencies
deps:
	@echo "--> Installing dependencies"
	@go mod download
	@go mod tidy
	@go run scripts/go-mod-tidy-all.go
.PHONY: deps

tidy-all:
	@go run scripts/tidy/go-mod-tidy-all.go
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
	@go vet ./...
.PHONY: vet

## mock-gen: generate mocks of external (commetbft) types
mock-gen:
	@echo "-> Generating mocks"
	mockery --output da/mocks --srcpkg github.com/rollkit/rollkit/core/da --name DA
	mockery --output test/mocks --srcpkg github.com/rollkit/rollkit/pkg/store --name Store
.PHONY: mock-gen
