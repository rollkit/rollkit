## run-n: Run 'n' nodes where 'n' is specified by the NODES variable (default: 1)
## Usage: make run-n NODES=3
run-single: build build-da
	@go run scripts/local/single.go --nodes=$(or $(NODES),1)
.PHONY: run-n
