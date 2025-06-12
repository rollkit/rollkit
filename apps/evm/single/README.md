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
      --evm.genesis-hash 0x593cca87d359c3d0fdc9b67a43f92e1c8eb0da113620225f476eee31a3920e46 \
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

# Rollkit EVM Full Node

./evm-single start --home /Users/manav/.evm-single-full-node --evm.jwt-secret $(cat ../../../execution/evm/docker/jwttoken/jwt.hex)       --evm.genesis-hash 0x593cca87d359c3d0fdc9b67a43f92e1c8eb0da113620225f476eee31a3920e46 --rollkit.rpc.address=127.0.0.1:46657 --rollkit.p2p.listen_address=/ip4/127.0.0.1/tcp/7677 --rollkit.p2p.peers=/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWHDaqNYmQHGDCkbjUNwmGA8fVJKnRzpXfBq4gwPktHrcT --evm.engine-url http://localhost:8561 --evm.eth-url http://localhost:8555