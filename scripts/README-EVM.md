# EVM Node Scripts

This directory contains Go scripts for running Rollkit EVM nodes.

## run-evm-nodes.go

A comprehensive script that automates the setup and execution of two EVM nodes:

- A sequencer node (aggregator)
- A full node that syncs from the sequencer

### Prerequisites

- Go 1.20 or later
- Docker installed and running
- OpenSSL (for JWT generation)
- Built binaries (script will build if missing):
  - `evm-single` binary
  - `local-da` binary

### Usage

```bash
# Build the script
go build -tags run_evm -o run-evm-nodes scripts/run-evm-nodes.go

# Run with default settings
./run-evm-nodes

# Run with custom options
./run-evm-nodes --clean-on-exit=false --log-level=debug
```

### Command Line Flags

- `--clean-on-exit` (default: true) - Remove node directories on exit
- `--log-level` (default: info) - Log level (debug, info, warn, error)

### What the Script Does

1. **Setup Phase**:
   - Generates JWT token for EVM authentication
   - Builds required binaries if not present
   - Creates necessary directories

2. **Infrastructure**:
   - Starts Local DA service on port 7980
   - Launches two Lumen (Reth) instances via Docker:
     - Sequencer EVM: ports 8545 (RPC), 8551 (Engine), 8546 (WS)
     - Full Node EVM: ports 8555 (RPC), 8561 (Engine), 8556 (WS)

3. **Rollkit Nodes**:
   <!-- markdown-link-check-disable -->
   - Initializes and starts the sequencer node:
     - RPC: <http://localhost:36657>
     - P2P: 127.0.0.1:7676
   - Initializes and starts the full node:
     - RPC: <http://localhost:46657>
     - P2P: 127.0.0.1:7677
     - Automatically connects to sequencer via P2P
     <!-- markdown-link-check-enable -->

4. **Monitoring**:
   - Monitors all processes for unexpected exits
   - Handles graceful shutdown on SIGINT/SIGTERM

### Architecture

```ascii
┌─────────────────┐     ┌─────────────────┐
│   Sequencer     │     │   Full Node     │
│  (Aggregator)   │◄────┤                 │
│                 │ P2P │                 │
└────────┬────────┘     └────────┬────────┘
         │                       │
         │ Engine API            │ Engine API
         │                       │
┌────────▼────────┐     ┌────────▼────────┐
│   Lumen EVM     │     │   Lumen EVM     │
│  (Sequencer)    │     │  (Full Node)    │
└────────┬────────┘     └────────┬────────┘
         │                       │
         └───────────┬───────────┘
                     │
              ┌──────▼──────┐
              │  Local DA   │
              │  (Port 7980)│
              └─────────────┘
```

### Cleanup

The script performs comprehensive cleanup on exit:

- Gracefully terminates all processes
- Stops and removes Docker containers
- Removes node directories (if --clean-on-exit=true)
- Cleans up JWT token files

### Troubleshooting

1. **Port conflicts**: Ensure the following ports are available:
   - 7980 (Local DA)
   - 7676-7677 (P2P)
   - 8545, 8551, 8546 (Sequencer EVM)
   - 8555, 8561, 8556 (Full Node EVM)
   - 36657, 46657 (Rollkit RPC)

2. **Docker issues**: Make sure Docker daemon is running and you have permissions

3. **Build failures**: Check that you're in the correct directory and have Go installed

4. **Connection issues**: The script waits for services to initialize, but you may need to adjust timeouts in some environments

### Development

The script follows Rollkit's patterns:

- Uses build tags for conditional compilation
- Implements comprehensive process management
- Provides detailed logging
- Handles errors gracefully
- Cleans up resources properly
