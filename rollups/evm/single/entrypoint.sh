#!/bin/sh
set -e

cd /usr/bin

sleep 5

./evm-single init --rollkit.node.aggregator=true --rollkit.signer.passphrase $EVM_SIGNER_PASSPHRASE

exec ./evm-single start \
  --evm.jwt-secret $EVM_JWT_SECRET \
  --evm.genesis-hash $EVM_GENESIS_HASH \
  --evm.engine-url $EVM_ENGINE_URL \
  --evm.eth-url $EVM_ETH_URL \
  --rollkit.node.block_time $EVM_BLOCK_TIME \
  --rollkit.node.aggregator=true \
  --rollkit.signer.passphrase $EVM_SIGNER_PASSPHRASE 