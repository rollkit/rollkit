# Evolve EVM Single Sequencer

This directory contains the implementation of a single EVM sequencer using Ev-node.

## Prerequisites

- Go 1.20 or later
- Docker and Docker Compose

## Starting the Sequencer Node

1. Both EVM and DA layers must be running before starting the sequencer
   1. For the EVM layer, Reth can be conveniently run using `docker compose` from <path_to>/execution/evm/docker.
   2. For the DA layer, local-da can be built and run from the `ev-node/tools/local-da` directory.

2. Build the sequencer:

   ```bash
   go build -o evm .
   ```

3. Initialize the sequencer:

   ```bash
   ./evm init --rollkit.node.aggregator=true --rollkit.signer.passphrase secret
   ```

4. Start the sequencer:

   ```bash
   ./evm start \
     --evm.jwt-secret $(cat <path_to>/execution/evm/docker/jwttoken/jwt.hex) \
     --evm.genesis-hash 0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503 \
     --rollkit.node.block_time 1s \
     --rollkit.node.aggregator=true \
     --rollkit.signer.passphrase secret
   ```

Share your `genesis.json` with other node operators. Add `da_start_height` field corresponding to the first DA included block of the chain (can be queried on the sequencer node).

Note: Replace `<path_to>` with the actual path to the rollkit repository. If you'd ever like to restart a fresh node, make sure to remove the originally created sequencer node directory using:

```bash
    rm -rf ~/.evm
```

## Configuration

The sequencer can be configured using various command-line flags. The most important ones are:

- `--rollkit.node.aggregator`: Set to true to run in sequencer mode
- `--rollkit.signer.passphrase`: Passphrase for the signer
- `--evm.jwt-secret`: JWT secret for EVM communication
- `--evm.genesis-hash`: Genesis hash of the EVM chain
- `--rollkit.node.block_time`: Block time for the Rollkit node

## Rollkit EVM Full Node

1. The sequencer must be running before starting any Full Node. You can run the EVM layer of the Full Node using `docker-compose -f docker-compose-full-node.yml` from <path_to>/execution/evm/docker.

2. Initialize the full node:

   ```bash
   ./evm init --home ~/.evm-full-node
   ```

3. Copy the genesis file from the sequencer node:

   ```bash
   cp ~/.evm/config/genesis.json ~/.evm-full-node/config/genesis.json
   ```

   Verify the `da_start_height` value in the genesis file is set. If not, ask the chain developer to share it.

4. Identify the sequencer node's P2P address from its logs. It will look similar to:

   ```sh
   1:55PM INF listening on address=/ip4/127.0.0.1/tcp/7676/p2p/12D3KooWJ1J5W7vpHuyktcvc71iuduRgb9pguY9wKMNVVPwweWPk module=main
   ```

   Create an environment variable with the P2P address:

   ```bash
   export P2P_ID="12D3KooWJbD9TQoMSSSUyfhHMmgVY3LqCjxYFz8wQ92Qa6DAqtmh"
   ```

5. Start the full node:

   ```bash
   ./evm start \
      --home ~/.evm-full-node \
      --evm.jwt-secret $(cat <path_to>/execution/evm/docker/jwttoken/jwt.hex) \
      --evm.genesis-hash 0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503 \
      --rollkit.rpc.address=127.0.0.1:46657 \
      --rollkit.p2p.listen_address=/ip4/127.0.0.1/tcp/7677 \
      --rollkit.p2p.peers=/ip4/127.0.0.1/tcp/7676/p2p/$P2P_ID \
      --evm.engine-url http://localhost:8561 \
      --evm.eth-url http://localhost:8555
   ```

If you'd ever like to restart a fresh node, make sure to remove the originally created full node directory using:

```bash
    rm -rf ~/.evm-full-node
```

## Force Inclusion API

The EVM app includes a Force Inclusion API server that exposes an Ethereum-compatible JSON-RPC endpoint for submitting transactions directly to the DA layer for force inclusion.

When enabling this server, the node operator will be paying for the gas costs of the transactions submitted through the API on the DA layer. The application costs are still paid by the transaction signer. With `--evnode.da.tx_signer.enabled`, those submissions are signed with the `da.tx_signer` key on any node role, so the key must be funded.

### Enabling the Force Inclusion API

To enable the Force Inclusion API server, add the following flag when starting the node:

```bash
./evm start \
  --force-inclusion-server="127.0.0.1:8547" \
  ... other flags ...
```

### Configuration Flag

- `--force-inclusion-server`: Address for the force inclusion API server (e.g. `127.0.0.1:8547`). If set, enables the server for direct DA submission

### Usage

Once enabled, the server exposes a standard Ethereum JSON-RPC endpoint that accepts `eth_sendRawTransaction` calls:

```bash
# Send a raw transaction for force inclusion
curl -X POST http://127.0.0.1:8547 \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "eth_sendRawTransaction",
    "params": ["0x02f873..."]
  }'
```

The transaction will be submitted directly to the DA layer in the force inclusion namespace, bypassing the normal mempool. The response returns a pseudo-transaction hash based on the DA height:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0x0000000000000000000000000000000000000000000000000000000000000064"
}
```

### Using with Spamoor

You can use this endpoint with [spamoor](https://github.com/ethpandaops/spamoor) for force inclusion testing:

```bash
spamoor \
  --rpc-url http://127.0.0.1:8547 \
  --private-key <your-private-key> \
  ... other spamoor flags ...
```

### Force Inclusion Timing

Transactions submitted to the Force Inclusion API are included in the chain at specific DA heights based on the `da_epoch_forced_inclusion` configuration in `genesis.json`. The API logs will show when the transaction will be force included:

```text
INF transaction successfully submitted to DA layer da_height=100
INF transaction will be force included blocks_until_inclusion=8 inclusion_at_height=110
```
