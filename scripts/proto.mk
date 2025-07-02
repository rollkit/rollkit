DOCKER := $(shell which docker)
DOCKER_BUF := $(DOCKER) run --rm -v $(CURDIR):/workspace --workdir /workspace bufbuild/buf:latest

## proto-gen: Generate protobuf files. Requires docker.
proto-gen:
	@echo "--> Generating Protobuf files"
	buf generate --path="./proto/rollkit" --template="buf.gen.yaml" --config="buf.yaml"
.PHONY: proto-gen

## proto-lint: Lint protobuf files. Requires docker.
proto-lint:
	@echo "--> Linting Protobuf files"
	@$(DOCKER_BUF) lint --error-format=json
.PHONY: proto-lint

## rust-proto-gen: Generate Rust protobuf files
rust-proto-gen:
	@echo "--> Generating Rust protobuf files"
	@cd client/crates/rollkit-types && cargo build
.PHONY: rust-proto-gen

## rust-proto-check: Check if Rust protobuf files are up to date
rust-proto-check:
	@echo "--> Checking Rust protobuf files"
	@rm -rf client/crates/rollkit-types/src/proto/*.rs
	@cd client/crates/rollkit-types && cargo build
	@if ! git diff --exit-code client/crates/rollkit-types/src/proto/; then \
		echo "Error: Generated Rust proto files are not up to date."; \
		echo "Run 'make rust-proto-gen' and commit the changes."; \
		exit 1; \
	fi
.PHONY: rust-proto-check
