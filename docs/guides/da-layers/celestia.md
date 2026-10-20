# Celestia

This guide covers connecting your Evolve chain to Celestia for production data availability.

## Prerequisites

- Completed an Evolve quickstart tutorial
- Familiarity with running a Celestia light node

## Running a Celestia Light Node

Before starting your Evolve chain, you need a Celestia light node running and synced.

Follow the [Celestia light node documentation](https://docs.celestia.org/how-to-guides/light-node) to install, initialize, and start a light node for your target network (Arabica, Mocha, or Mainnet).

## Configuring Evolve for Celestia

### Required Configuration

The following flags are required to connect to Celestia:

| Flag | Description |
|------|-------------|
| `--evnode.da.address` | Celestia node RPC endpoint |
| `--evnode.da.auth_token` | JWT authentication token |
| `--evnode.da.header_namespace` | Namespace for block headers |
| `--evnode.da.data_namespace` | Namespace for transaction data |

### Get DA Block Height

Query the current DA height to set as your starting point:

```bash
DA_BLOCK_HEIGHT=$(celestia header network-head | jq -r '.result.header.height')
echo "Your DA_BLOCK_HEIGHT is $DA_BLOCK_HEIGHT"
```

### Get Authentication Token

Generate a write token for your light node:

**Arabica:**

```bash
AUTH_TOKEN=$(celestia light auth write --p2p.network arabica)
```

**Mocha:**

```bash
AUTH_TOKEN=$(celestia light auth write --p2p.network mocha)
```

**Mainnet:**

```bash
AUTH_TOKEN=$(celestia light auth write)
```

### Set Namespaces

Choose unique namespaces for your chain's headers and data:

```bash
DA_HEADER_NAMESPACE="my_chain_headers"
DA_DATA_NAMESPACE="my_chain_data"
```

The namespace values are automatically encoded by ev-node for Celestia compatibility.

You can use the same namespace for both headers and data, or separate them for optimized syncing (light clients can sync headers only).

### Set DA Address

Default Celestia light node port is 26658. **Note:** Connection to a celestia-node DA requires a websocket connection:

```bash
DA_ADDRESS=ws://localhost:26658
```

## Running Your Chain

Start your chain with Celestia configuration:

```bash
evnode start \
    --evnode.node.aggregator \
    --evnode.da.auth_token $AUTH_TOKEN \
    --evnode.da.header_namespace $DA_HEADER_NAMESPACE \
    --evnode.da.data_namespace $DA_DATA_NAMESPACE \
    --evnode.da.address $DA_ADDRESS
```

For Cosmos SDK chains:

```bash
appd start \
    --evnode.node.aggregator \
    --evnode.da.auth_token $AUTH_TOKEN \
    --evnode.da.header_namespace $DA_HEADER_NAMESPACE \
    --evnode.da.data_namespace $DA_DATA_NAMESPACE \
    --evnode.da.address $DA_ADDRESS
```

## Bring Your Own Key (Signed Submission)

By default ev-node calls `blob.Submit` on the Celestia node, which signs and pays with **the node's** keyring. Shared DA endpoints such as da-proxy have no funded account, so `blob.Submit` through them cannot succeed. With `da.tx_signer` enabled, ev-node holds the key itself: it signs each PayForBlobs locally and broadcasts it over Celestia consensus gRPC. Reads (retrieval, headers, proofs) still use `--evnode.da.address`.

This is supported by the `testapp` and `evm` binaries.

### da-proxy Endpoints

da-proxy fronts DA JSON-RPC and consensus gRPC behind one token with `write` scope:

| Network | DA JSON-RPC (`da.address`) | Consensus gRPC (`tx_signer.grpc_address`) |
|---|---|---|
| Mocha | `https://da.da-proxy-lb.celestia-mocha.com` | `grpc.da-proxy-lb.celestia-mocha.com:443` |
| Mainnet | `https://da.da-proxy-lb.lunaroasis.net` | `grpc.da-proxy-lb.lunaroasis.net:443` |

The token is sent as `Authorization: Bearer` on JSON-RPC and as `x-token` metadata on gRPC. Set it once in `--evnode.da.auth_token`; `tx_signer.grpc_auth_token` defaults to it.

### Create the Key

Use `cel-key` (shipped with celestia-node) to create or import a key into a keyring directory under the node home (default `<home>/keys`):

```bash
cel-key add evnode --keyring-backend file --keyring-dir ~/.evm/keys
# or import an existing mnemonic (BIP44 path m/44'/118'/0'/0/0):
cel-key add evnode --recover --keyring-backend file --keyring-dir ~/.evm/keys
```

The passphrase you choose (at least 8 characters) unlocks the keyring at startup. Provide it through `EVNODE_DA_KEYRING_PASSPHRASE` or a file passed via `--evnode.da.tx_signer.passphrase_file`; never on the command line. The `test` backend is unencrypted and only for local development.

### Fund the Key

Show the address and fund it from a faucet (testnets) or a TIA transfer (Mainnet):

```bash
cel-key show evnode -a --keyring-backend file --keyring-dir ~/.evm/keys
```

ev-node logs the address and balance when it starts. Watch the balance: once the key runs out of TIA, blob submission stops.

### Start the Node

```bash
export EVNODE_DA_KEYRING_PASSPHRASE=...   # or --evnode.da.tx_signer.passphrase_file
evm start \
    --evnode.node.aggregator \
    --evnode.da.address https://da.da-proxy-lb.celestia-mocha.com \
    --evnode.da.auth_token $DA_PROXY_TOKEN \
    --evnode.da.namespace $DA_HEADER_NAMESPACE \
    --evnode.da.data_namespace $DA_DATA_NAMESPACE \
    --evnode.da.tx_signer.enabled \
    --evnode.da.tx_signer.grpc_address grpc.da-proxy-lb.celestia-mocha.com:443 \
    --evnode.da.tx_signer.key_name evnode
```

The chain-id is taken from the gRPC endpoint. Startup fails if the key is missing from the keyring or its account does not exist on chain yet, so fund it first. See the [configuration reference](/learn/config#da-tx-signer) for every option.

### Notes

- `--evnode.da.signing_addresses` cannot be combined with `tx_signer`; one key signs every submission.
- Submit options such as `gas_price` and `max_gas_price` in `--evnode.da.submit_options` apply as before. Set `max_gas_price` to cap fees during gas spikes.
- A transaction that times out may still be committed later and posted again, as with node-side signing. Keep `da.block_time × da.mempool_ttl` at or above the confirmation window.

## Viewing Your Chain Data

Once running, you can view your chain's data on Celestia block explorers:

- [Celenium (Mocha)](https://mocha.celenium.io/)
- [Celenium (Mainnet)](https://celenium.io/)

Search by your namespace or account address to see submitted blobs.

## Configuration Options

### Gas Price

By default, ev-node uses automatic gas price detection. Keep the default unless you have an operational reason to override it:

```bash
--evnode.da.gas_price 0.01
```

Higher gas prices result in faster inclusion during congestion. Omit this flag to use the automatic default.

### Block Time

Set the expected DA block time (affects retry timing):

```bash
--evnode.da.block_time 6s
```

Celestia's block time is approximately 6 seconds.

### Multiple Signing Addresses

For high-throughput chains, use multiple signing addresses to avoid nonce conflicts:

```bash
--evnode.da.signing_addresses celestia1abc...,celestia1def...,celestia1ghi...
```

All addresses must be funded and loaded in the Celestia node's keyring.

## Funding Your Account

### Testnet (Mocha/Arabica)

Get testnet TIA from faucets:

- [Mocha Faucet](https://faucet.celestia-mocha.com/)
- [Arabica Faucet](https://faucet.celestia-arabica.com/)

### Mainnet

Purchase TIA and transfer to your Celestia light node address.

Check your address:

```bash
celestia state account-address
```

## Troubleshooting

### Out of Funds

If you see `Code: 19` errors, your account is out of TIA:

1. Fund your account
2. Increase gas price to unstick pending transactions
3. Restart your chain

See [Troubleshooting Guide](/guides/operations/troubleshooting) for details.

### Connection Refused

Verify your Celestia node is running:

```bash
curl http://localhost:26658/header/sync_state
```

### Token Expired

Regenerate your auth token:

```bash
celestia light auth write --p2p.network <network>
```

## See Also

- [Local DA Guide](/guides/da-layers/local-da) - Development setup
- [Troubleshooting](/guides/operations/troubleshooting) - Common issues
- [Configuration Reference](/reference/configuration/ev-node-config) - All DA options
