#!/bin/bash

# OP-Reth Setup Script for Rollkit
# This script sets up OP-Reth for use with Rollkit's EVM execution engine

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GENESIS_FILE="$SCRIPT_DIR/chain/genesis.json"
JWT_FILE="$SCRIPT_DIR/jwttoken/jwt.hex"

echo "Setting up OP-Reth for Rollkit..."

# Ensure directories exist
mkdir -p "$SCRIPT_DIR/jwttoken"
mkdir -p "$SCRIPT_DIR/chain"

# Generate JWT secret if it doesn't exist
if [ ! -f "$JWT_FILE" ]; then
    echo "Generating JWT secret..."
    openssl rand -hex 32 | tr -d '\n' > "$JWT_FILE"
    echo "JWT secret generated at: $JWT_FILE"
else
    echo "JWT secret already exists at: $JWT_FILE"
fi

# Verify genesis file exists
if [ ! -f "$GENESIS_FILE" ]; then
    echo "Error: Genesis file not found at $GENESIS_FILE"
    echo "Please ensure the genesis.json file exists before running this script."
    exit 1
fi

echo "Genesis file found at: $GENESIS_FILE"

# Test OP-Reth initialization
echo "Testing OP-Reth initialization..."
docker run --rm -v "$SCRIPT_DIR/chain:/chain" ghcr.io/paradigmxyz/op-reth:v1.4.6 \
    init --chain /chain/genesis.json --datadir /tmp/test-datadir > /dev/null 2>&1

if [ $? -eq 0 ]; then
    echo "✓ OP-Reth initialization test successful"
else
    echo "✗ OP-Reth initialization test failed"
    exit 1
fi

# Extract genesis hash for verification
echo "Extracting genesis hash..."
GENESIS_HASH=$(docker run --rm -v "$SCRIPT_DIR/chain:/chain" ghcr.io/paradigmxyz/op-reth:v1.4.6 \
    init --chain /chain/genesis.json --datadir /tmp/extract-datadir 2>&1 | \
    grep "Genesis block written" | \
    sed -n 's/.*hash=\(0x[a-fA-F0-9]*\).*/\1/p')

if [ -n "$GENESIS_HASH" ]; then
    echo "✓ Genesis hash: $GENESIS_HASH"
    echo "  Make sure to update this hash in your Rollkit configuration files:"
    echo "  - apps/evm/single/docker-compose.yml"
    echo "  - apps/evm/single/README.md"
    echo "  - execution/evm/execution_test.go"
else
    echo "✗ Failed to extract genesis hash"
    exit 1
fi

echo
echo "OP-Reth setup complete!"
echo "You can now start OP-Reth using:"
echo "  docker-compose up"
echo
echo "Configuration summary:"
echo "  - OP-Reth image: ghcr.io/paradigmxyz/op-reth:v1.4.6"
echo "  - Genesis file: $GENESIS_FILE"
echo "  - JWT secret: $JWT_FILE"
echo "  - Genesis hash: $GENESIS_HASH"
echo "  - Optimism features: enabled" 