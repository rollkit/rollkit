## install: Install rollkit CLI
run-1: build build-da
	@go run scripts/local/1node.go
.PHONY: run-1
