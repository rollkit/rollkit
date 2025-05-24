#!/bin/sh
set -e

cd /usr/bin

sleep 5

./evm-single init --rollkit.node.aggregator=true --rollkit.signer.passphrase $EVM_SIGNER_PASSPHRASE

# Conditionally add --rollkit.da.address if ROLLKIT_DA_ADDRESS is set
da_flag=""
if [ -n "$DA_ADDRESS" ]; then
  da_flag="--rollkit.da.address $DA_ADDRESS"
fi

# Conditionally add --rollkit.da.auth_token if ROLLKIT_DA_AUTH_TOKEN is set
da_auth_token_flag=""
if [ -n "$DA_AUTH_TOKEN" ]; then
  da_auth_token_flag="--rollkit.da.auth_token $DA_AUTH_TOKEN"
fi

# Conditionally add --rollkit.da.namespace if ROLLKIT_DA_NAMESPACE is set
da_namespace_flag=""
if [ -n "$DA_NAMESPACE" ]; then
  da_namespace_flag="--rollkit.da.namespace $DA_NAMESPACE"
fi

exec ./evm-single start \
  --evm.jwt-secret $EVM_JWT_SECRET \
  --evm.genesis-hash $EVM_GENESIS_HASH \
  --evm.engine-url $EVM_ENGINE_URL \
  --evm.eth-url $EVM_ETH_URL \
  --rollkit.node.block_time $EVM_BLOCK_TIME \
  --rollkit.node.aggregator=true \
  --rollkit.signer.passphrase $EVM_SIGNER_PASSPHRASE \
  $da_flag \
  $da_auth_token_flag \
  $da_namespace_flag 