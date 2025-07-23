# gRPC Single Sequencer App

This application runs a Rollkit node with a single sequencer that connects to a remote execution client via gRPC. It allows you to use any execution layer that implements the Rollkit execution gRPC interface.

## Overview

The gRPC single sequencer app provides:
- A Rollkit consensus node with single sequencer
- Connection to remote execution clients via gRPC
- Full data availability layer integration
- P2P networking capabilities

## Prerequisites

1. A running execution client that implements the Rollkit gRPC execution interface
2. Access to a data availability layer (e.g., local DA, Celestia)
3. Go 1.22 or higher

## Installation

From the repository root:

```bash
cd apps/grpc/single
go build -o grpc-single
```

## Usage

### 1. Initialize the Node

First, initialize the node configuration:

```bash
./grpc-single init --root-dir ~/.grpc-single
```

This creates the necessary configuration files and directories.

### 2. Configure the Node

Edit the configuration file at `~/.grpc-single/config/config.toml` to set your preferred parameters, or use command-line flags.

### 3. Start the Execution Service

Before starting the Rollkit node, ensure your gRPC execution service is running. For example:

```bash
# Example: Starting a test execution service
cd execution/grpc
go run examples/server/main.go
```

### 4. Run the Node

Start the Rollkit node with:

```bash
./grpc-single start \
  --root-dir ~/.grpc-single \
  --grpc-executor-url http://localhost:50051 \
  --da.address http://localhost:7980 \
  --da.auth-token your-da-token \
  --chain-id your-chain-id
```

## Command-Line Flags

### gRPC-specific Flags

- `--grpc-executor-url`: URL of the gRPC execution service (default: `http://localhost:50051`)

### Common Rollkit Flags

- `--root-dir`: Root directory for config and data (default: `~/.grpc-single`)
- `--chain-id`: The chain ID for your rollup
- `--da.address`: Data availability layer address
- `--da.auth-token`: Authentication token for DA layer
- `--da.namespace`: Namespace for DA layer (optional)
- `--p2p.listen-address`: P2P listen address (default: `/ip4/0.0.0.0/tcp/26656`)
- `--block-time`: Time between blocks (default: `1s`)

## Example: Running with Local DA

1. Start the local DA service:
```bash
cd da/cmd/local-da
go run main.go
```

2. Start your gRPC execution service:
```bash
# Your execution service implementation
```

3. Initialize and run the node:
```bash
./grpc-single init --root-dir ~/.grpc-single
./grpc-single start \
  --root-dir ~/.grpc-single \
  --grpc-executor-url http://localhost:50051 \
  --da.address http://localhost:7980 \
  --chain-id test-chain
```

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────┐
│   Rollkit Node  │────▶│ gRPC Execution   │────▶│  Execution  │
│ (Single Seqr)   │◀────│     Client       │◀────│   Service   │
└─────────────────┘     └──────────────────┘     └─────────────┘
         │                                                │
         │                                                │
         ▼                                                ▼
┌─────────────────┐                              ┌─────────────┐
│       DA        │                              │    State    │
│     Layer       │                              │   Storage   │
└─────────────────┘                              └─────────────┘
```

## Development

### Building from Source

```bash
go build -o grpc-single
```

### Running Tests

```bash
go test ./...
```

## Troubleshooting

### Connection Refused

If you see "connection refused" errors, ensure:
1. Your gRPC execution service is running
2. The execution service URL is correct
3. No firewall is blocking the connection

### DA Layer Issues

If you have issues connecting to the DA layer:
1. Verify the DA service is running
2. Check the authentication token
3. Ensure the namespace exists (if using Celestia)

## See Also

- [Rollkit Documentation](https://rollkit.dev)
- [gRPC Execution Interface](../../execution/grpc/README.md)
- [Single Sequencer Documentation](../../sequencers/single/README.md)