# Rollkit EVM Single Sequencer

This directory contains the implementation of a single EVM sequencer using Rollkit.

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose

## Starting the Sequencer Node

1. Both EVM and DA layers must be running before starting the sequencer
   1. For the EVM layer, Reth can be conveniently run using `docker compose` from <path_to>/execution/evm/docker.
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
      --evm.genesis-hash 0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503 \
      --rollkit.node.block_time 1s \
      --rollkit.node.aggregator=true \
      --rollkit.signer.passphrase secret
    ```

Note: Replace `<path_to>` with the actual path to the rollkit repository.

## Configuration

The sequencer can be configured using various command-line flags. The most important ones are:

- `--rollkit.node.aggregator`: Set to true to run in aggregator mode
- `--rollkit.signer.passphrase`: Passphrase for the signer
- `--evm.jwt-secret`: JWT secret for EVM communication
- `--evm.genesis-hash`: Genesis hash of the EVM chain
- `--rollkit.node.block_time`: Block time for the Rollkit node

## Rollkit EVM Full Node

1. The sequencer must be running before starting any Full Node. You can run the EVM layer of the Full Node using `docker-compose -f docker-compose-full-node.yml` from <path_to>/execution/evm/docker.

2. Initialize the full node:
    ```bash
    ./evm-single init --home ~/.evm-single-full-node
    ```
3. Copy the genesis file from the sequencer node:
    ```bash
    cp ~/.evm-single/config/genesis.json ~/.evm-single-full-node/config/genesis.json 
    ```
4. Identify the sequencer node's P2P address from its logs. It will look similar to:
    ```
    1:55PM INF listening on address=/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWJ1J5W7vpHuyktcvc71iuduRgb9pguY9wKMNVVPwweWPk module=main
    ```

    Create an environment variable with the P2P address:
    ```bash
    export P2P_ID="12D3KooWJbD9TQoMSSSUyfhHMmgVY3LqCjxYFz8wQ92Qa6DAqtmh"
    ```

5. Start the full node:

    ```bash
    ./evm-single start \ 
       --home /Users/manav/.evm-single-full-node \
       --evm.jwt-secret $(cat <path_to>/execution/evm/docker/jwttoken/jwt.hex) \
       --evm.genesis-hash 0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503 \
       --rollkit.rpc.address=127.0.0.1:46657 \
       --rollkit.p2p.listen_address=/ip4/127.0.0.1/tcp/7677 \
       --rollkit.p2p.peers=/ip4/127.0.0.1/tcp/7676/$P2P_ID \
       --evm.engine-url http://localhost:8561 \
       --evm.eth-url http://localhost:8555
    ```
