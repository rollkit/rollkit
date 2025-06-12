# OP-Reth Configuration for Rollkit

This directory has been configured to use **OP-Reth** (Optimism Reth) instead of standard Reth for enhanced Optimism rollup compatibility.

## What Changed

### 1. Docker Configuration (`docker-compose.yml`)
- **Image**: Changed from `ghcr.io/paradigmxyz/reth:v1.4.6` to `ghcr.io/paradigmxyz/op-reth:v1.4.6`
- **Container Name**: Changed from `reth` to `op-reth`
- **Command**: Changed from `reth node` to `op-reth node`
- **Added OP-Reth specific flags**:
  - `--rollup.disable-tx-pool-gossip`: Disables transaction pool gossiping for rollup usage
  - `--rollup.enable-genesis-walkback`: Enables genesis walkback for chain verification
  - `--rollup.discovery.v4`: Enables discovery v4 protocol for peer discovery

### 2. Genesis Configuration (`chain/genesis.json`)
- **Added Optimism configuration section**:
  ```json
  "optimism": {
    "eip1559Elasticity": 6,
    "eip1559Denominator": 50,
    "eip1559DenominatorCanyon": 250
  }
  ```
- **New Genesis Hash**: `0x593cca87d359c3d0fdc9b67a43f92e1c8eb0da113620225f476eee31a3920e46`
  (This replaces the old hash: `0x2b8bbb1ea1e04f9c9809b4b278a8687806edc061a356c7dbc491930d8e922503`)

### 3. Configuration Files Updated
The following files have been updated with the new genesis hash:
- `apps/evm/single/docker-compose.yml` - EVM_GENESIS_HASH environment variable
- `apps/evm/single/README.md` - Command line examples
- `execution/evm/execution_test.go` - Test constants
- `execution/evm/docker/docker-compose-parallel.yml` - Parallel setup

## Why OP-Reth?

OP-Reth provides several advantages over standard Reth for rollup usage:

1. **Optimism-specific features**: Built-in support for Optimism rollup transaction types and gas calculations
2. **Enhanced Engine API**: Optimized Engine API implementation for rollup sequencing
3. **Transaction pool optimizations**: Better handling of rollup-specific transaction flows
4. **Reduced resource usage**: Optimized for rollup environments with specific discovery and gossiping settings

## Usage

### Quick Start
1. Run the setup script to initialize everything:
   ```bash
   ./setup-op-reth.sh
   ```

2. Start OP-Reth:
   ```bash
   docker-compose up
   ```

### Manual Setup
1. Ensure the genesis file exists: `chain/genesis.json`
2. Generate JWT secret if needed:
   ```bash
   mkdir -p jwttoken
   openssl rand -hex 32 | tr -d '\n' > jwttoken/jwt.hex
   ```
3. Start the services:
   ```bash
   docker-compose up
   ```

## Configuration Details

### OP-Reth Flags Explained
- `--rollup.disable-tx-pool-gossip`: Prevents transaction pool gossiping between peers, which is unnecessary for most rollup setups where transactions are submitted directly to the sequencer
- `--rollup.enable-genesis-walkback`: Allows the node to walk back to genesis during startup to verify chain integrity
- `--rollup.discovery.v4`: Enables discovery v4 protocol, which is the preferred discovery method for OP Stack chains

### Genesis Configuration
The Optimism configuration section in genesis.json contains:
- `eip1559Elasticity`: Controls EIP-1559 elasticity (set to 6 for Optimism)
- `eip1559Denominator`: Base fee update denominator (set to 50)
- `eip1559DenominatorCanyon`: Canyon fork denominator (set to 250)

## Compatibility

This setup is compatible with:
- Rollkit's EVM execution engine
- Standard Ethereum JSON-RPC API
- Engine API for block building
- Optimism rollup features

## Troubleshooting

### Common Issues

1. **"forkchoice update failed: Invalid params"**
   - Ensure you're using the correct genesis hash in your Rollkit configuration
   - Verify the genesis.json file contains the Optimism configuration section

2. **JWT Authentication Errors**
   - Ensure the JWT secret file exists and is readable by the container
   - Verify the JWT secret path is correctly mounted in docker-compose.yml

3. **Genesis Block Mismatch**
   - If you see genesis hash mismatches, regenerate the genesis hash using:
     ```bash
     docker run --rm -v "$(pwd)/chain:/chain" ghcr.io/paradigmxyz/op-reth:v1.4.6 \
       init --chain /chain/genesis.json --datadir /tmp/datadir
     ```
   - Update all configuration files with the new hash

### Logs and Debugging
- Container logs: `docker-compose logs op-reth`
- OP-Reth logs are written to the `logs` volume
- Enable verbose logging by adding `--verbosity=4` to the command

## Migration from Standard Reth

If migrating from standard Reth:
1. Stop the existing Reth container
2. Update docker-compose.yml to use OP-Reth image and commands
3. Update genesis.json with Optimism configuration
4. Update all references to the old genesis hash
5. Restart with the new configuration

## Further Reading

- [OP-Reth Documentation](https://reth.rs/run/optimism.html)
- [Optimism Rollup Specification](https://specs.optimism.io/)
- [Rollkit Documentation](https://rollkit.dev/) 