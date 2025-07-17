## run-n: Run 'n' nodes where 'n' is specified by the NODES variable (default: 1)
## Usage: make run-n NODES=3
run-n: build build-da
	@go run -tags=run scripts/run.go --nodes=$(or $(NODES),1)
.PHONY: run-n

## run-evm-nodes: Run EVM nodes (one sequencer and one full node)
## Usage: make run-evm-nodes
run-evm-nodes: build-da build-evm-single
	@echo "Starting EVM nodes..."
	@go run -tags=run_evm scripts/run-evm-nodes.go --nodes=$(or $(NODES),1)
.PHONY: run-evm-nodes
