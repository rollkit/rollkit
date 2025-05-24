# based-rollkit

🚀 **Based Rollkit Node** is a sovereign application node powered by [Rollkit](https://rollkit.dev), configured to operate in *based sequencing mode*. This mode leverages Celestia as a base layer for ordering transactions and data availability.

This implementation supports EVM execution via `go-execution-evm` and allows configuration for both **chain DA** and **based sequencing DA** independently.

---

## 📦 Features

- Modular Rollkit node launcher
- Supports **based sequencing** via Celestia or dummy DA
- Integrated with EVM execution (reth + op-geth compatible)
- Configurable DA layers (chain and based)
- Lightweight node initialization
- RPC and P2P setup out of the box

---

## 🔧 Usage

```bash
based start [flags]
```

### Example (Mocha Network with dummy DA)

```bash
based start \
  --home ~/.rollkit/based \
  --evm.eth-url http://localhost:8545 \
  --evm.engine-url http://localhost:8551 \
  --evm.jwt-secret /path/to/jwt.hex \
  --evm.genesis-hash 0xabc123... \
  --evm.fee-recipient 0xYourAddress \
  --based.url http://localhost:26658 \
  --based.namespace 0102030405060708090a \
  --based.start-height 0 \
  --based.max-height-drift 1 \
  --based.gas-multiplier 1.0 \
  --based.gas-price 1.0
```

---

## 💠 Command-Line Flags

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

### 📁 Node Configuration

| Flag | Description |
|------|-------------|
| `--home` | Directory to store config, keys, and state (default `~/.rollkit/based`) |

---

## 📡 Ports

- `26657` – Core RPC (not used in light mode)
- `26658` – Light node RPC
- `26659` – P2P
- `9090` – Prometheus metrics

---

## 🧪 Development

- To test with a dummy DA, leave `--based.auth` empty.
- Namespace ID must be hex-encoded (e.g., use `hex.EncodeToString` in Go).
- Supports both Celestia RPC-based DA and mock in-memory DA for testing.

---

## 🚠 Troubleshooting

- Ensure correct permissions on the `--home` directory for Docker.
- Ensure JWT secrets and engine URLs match reth/op-geth config.
- Use `--based.max-height-drift` to control batching latency.

---
