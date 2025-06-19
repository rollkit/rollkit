# Rollkit EVM Based Sequencer

This directory contains the implementation of a based EVM sequencer using Rollkit.

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose
- Access to the go-execution-evm repository op-geth branch

## Starting the Aggregator Node

1. Both EVM and DA layers must be running before starting the aggregator
   1. For the EVM layer, Reth can be conveniently run using `docker compose` from the go-execution-evm repository.
   2. For the DA layer, local-da can be built and run from the `rollkit/da/cmd/local-da` directory.

2. Build the sequencer:

    ```bash
    go build -o evm-based .
    ```
  
3. Initialize the sequencer:

    ```bash
    ./evm-based init --rollkit.node.aggregator=true --rollkit.signer.passphrase secret
    ```

4. Start the sequencer:

  ```bash
  ./evm-based start \
    --evm.jwt-secret $(cat <path_to>/go-execution-evm/docker/jwttoken/jwt.hex) \
    --evm.genesis-hash 0x0a962a0d163416829894c89cb604ae422323bcdf02d7ea08b94d68d3e026a380 \
    --rollkit.node.block_time 1s \
    --rollkit.node.aggregator=true \
    --rollkit.signer.passphrase secret \
    --based.url http://localhost:26658 \
    --based.namespace 0102030405060708090a \
    --based.start-height 0 \
    --based.max-height-drift 1 \
    --based.gas-multiplier 1.0 \
    --based.gas-price 1.0
  ```

Note: Replace `<path_to>` with the actual path to your go-execution-evm repository.

## Configuration

### 🧐 EVM Execution Configuration

| Flag | Description |
|------|-------------|
| `--evm.eth-url` | Ethereum JSON-RPC URL (default `http://localhost:8545`) |
| `--evm.engine-url` | Engine API URL (default `http://localhost:8551`) |
| `--evm.jwt-secret` | JWT secret file path for the Engine API |
| `--evm.genesis-hash` | Genesis block hash of the chain |
| `--evm.fee-recipient` | Address to receive priority fees (used by the execution layer) |

---

### ⚙️ Based Sequencer Configuration

| Flag | Description |
|------|-------------|
| `--based.url` | URL for Celestia light node or other DA endpoint (default `http://localhost:26658`) |
| `--based.auth` | Auth token for based DA layer |
| `--based.namespace` | Hex-encoded namespace ID for submitting transactions (e.g., `010203...`) |
| `--based.start-height` | Starting DA height for fetching transactions (default `0`) |
| `--based.max-height-drift` | Max number of DA heights to look ahead during batching (default `1`) |
| `--based.gas-multiplier` | Gas multiplier to apply on DA submission (default `1.0`) |
| `--based.gas-price` | Base gas price to use during DA submission (default `1.0`) |

---
