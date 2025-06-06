# Rollkit EVM Single Sequencer

This directory contains the implementation of a single EVM sequencer using Rollkit.

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose
- Access to the go-execution-evm repository

## Starting the Aggregator Node

1. Both EVM and DA layers must be running before starting the aggregator
   1. For the EVM layer, Reth can be conveniently run using `docker compose` from the go-execution-evm repository.
   2. For the DA layer, local-da can be built and run from the `rollkit/da/cmd/local-da` directory.

2. Build the sequencer:

    ```bash
    go build -o evm-single .
    ```
  
3. Initialize the sequencer:

    ```bash
    ./evm-single init --rollkit.node.aggregator=true --rollkit.signer.passphrase secret
    ```

4. Start the sequencer:

    ```bash
    ./evm-single start \
      --evm.jwt-secret $(cat <path_to>/execution/evm/docker/jwttoken/jwt.hex) \
      --evm.genesis-hash 0xe720f8ec96a43a741b1ab34819acfeb029ce4f083fe73c5a08c1f6a7b17a8568 \
      --rollkit.node.block_time 1s \
      --rollkit.node.aggregator=true \
      --rollkit.signer.passphrase secret
    ```

Note: Replace `<path_to>` with the actual path to your go-execution-evm repository.

## Configuration

The sequencer can be configured using various command-line flags. The most important ones are:

- `--rollkit.node.aggregator`: Set to true to run in aggregator mode
- `--rollkit.signer.passphrase`: Passphrase for the signer
- `--evm.jwt-secret`: JWT secret for EVM communication
- `--evm.genesis-hash`: Genesis hash of the EVM chain
- `--rollkit.node.block_time`: Block time for the Rollkit node
