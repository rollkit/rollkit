## run-n: Run 'n' nodes where 'n' is specified by the NODES variable (default: 1)
## Usage: make run-n NODES=3
run-n: build build-da
	@go run -tags=run scripts/run.go --nodes=$(or $(NODES),1)
.PHONY: run-n
