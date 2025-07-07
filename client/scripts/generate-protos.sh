#!/bin/bash
set -euo pipefail

# Script to generate proto files with the same version as CI
# This ensures consistent output between local development and CI

PROTOC_VERSION="25.1"
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/../.." && pwd )"

echo "Checking protoc version..."
CURRENT_VERSION=$(protoc --version | cut -d' ' -f2)

if [ "$CURRENT_VERSION" != "$PROTOC_VERSION" ]; then
    echo "Warning: Your protoc version ($CURRENT_VERSION) differs from CI version ($PROTOC_VERSION)"
    echo "This may cause differences in generated files."
    echo ""
    echo "To install the correct version:"
    echo "  - macOS: brew install protobuf@$PROTOC_VERSION"
    echo "  - Linux: Download from https://github.com/protocolbuffers/protobuf/releases/tag/v$PROTOC_VERSION"
    echo ""
    read -p "Continue anyway? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

echo "Cleaning existing generated files..."
rm -f "$PROJECT_ROOT/client/crates/rollkit-types/src/proto/"*.rs

echo "Generating proto files..."
cd "$PROJECT_ROOT/client/crates/rollkit-types"
cargo build

echo "Proto generation complete!"
echo ""
echo "If you see differences in git, it may be due to protoc version differences."
echo "CI uses protoc version $PROTOC_VERSION"