SHELL=/usr/bin/env bash
PROJECTNAME=$(shell basename "$(PWD)")

## help: Get more info on make commands.
help: Makefile
	@echo " Choose a command run in "$(PROJECTNAME)":"
	@sed -n 's/^##//p' $< | column -t -s ':' |  sed -e 's/^/ /'
.PHONY: help

## deps: install dependencies.
deps:
	@echo "--> Installing Dependencies"
	@go mod download
.PHONY: deps

## fmt: Formats only *.go (excluding *.pb.go *pb_test.go). Runs `gofmt & goimports` internally.
fmt: sort-imports
	@find . -name '*.go' -type f -not -path "*.git*" -not -name '*.pb.go' -not -name '*pb_test.go' | xargs gofmt -w -s
	@find . -name '*.go' -type f -not -path "*.git*"  -not -name '*.pb.go' -not -name '*pb_test.go' | xargs goimports -w -local github.com/celestiaorg
	@go mod tidy
	@cfmt -w -m=100 ./...
	@markdownlint --fix --quiet --config .markdownlint.yaml .
.PHONY: sort-imports

## lint: Linting *.go files using golangci-lint. Look for .golangci.yml for the list of linters.
lint: lint-imports
	@echo "--> Running linter"
	@golangci-lint run
	@markdownlint --config .markdownlint.yaml '**/*.md'
	@cfmt -m=100 ./...
.PHONY: lint

## test-all: Running both unit and swamp tests
test:
	@echo "--> Running all tests without data race detector"
	@go test ./...
	@echo "--> Running all tests with data race detector"
	@go test -race ./...
.PHONY: test

## cover: generate to code coverage report.
cover:
	@echo "--> Generating Code Coverage"
	@go install github.com/ory/go-acc@latest
	@go-acc -o coverage.txt `go list ./... | grep -v nodebuilder/tests` -- -v
.PHONY: cover

## benchmark: Running all benchmarks
benchmark:
	@echo "--> Running benchmarks"
	@go test -run="none" -bench=. -benchtime=100x -benchmem ./...
.PHONY: benchmark

lint-imports:
	@echo "--> Running imports linter"
	@for file in `find . -type f -name '*.go'`; \
		do goimports-reviser -list-diff -set-exit-status -company-prefixes "github.com/celestiaorg"  -project-name "github.com/celestiaorg/"$(PROJECTNAME)"" -output stdout $$file \
		 || exit 1;  \
    done;
.PHONY: lint-imports

sort-imports:
	@for file in `find . -type f -name '*.go'`; \
    		do goimports-reviser -company-prefixes "github.com/celestiaorg"  -project-name "github.com/celestiaorg/"$(PROJECTNAME)"" $$file \
    		 || exit 1;  \
        done;
.PHONY: sort-imports

pb-gen:
	@echo '--> Generating protobuf'
	@for dir in $(PB_PKGS); \
		do for file in `find $$dir -type f -name "*.proto"`; \
			do protoc -I=. -I=${PB_CORE}/proto/ -I=${PB_GOGO} -I=${PB_CELESTIA_APP}/proto --gogofaster_out=paths=source_relative:. $$file; \
			echo '-->' $$file; \
		done; \
	done;
.PHONY: pb-gen
