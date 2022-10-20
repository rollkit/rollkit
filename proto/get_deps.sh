#!/usr/bin/env bash

cd "$(dirname "${BASH_SOURCE[0]}")"

TM_VERSION=abci_fraud_proofs
TM_PROTO_URL=https://raw.githubusercontent.com/celestiaorg/tendermint/$TM_VERSION/proto/tendermint

TM_PROTO_FILES=(
  abci/types.proto
  version/types.proto
  types/types.proto
  types/evidence.proto
  types/params.proto
  types/validator.proto
  state/types.proto
  crypto/proof.proto
  crypto/keys.proto
  libs/bits/types.proto
  p2p/types.proto
)

echo Fetching protobuf dependencies from Tendermint $TM_VERSION
for FILE in "${TM_PROTO_FILES[@]}"; do
  echo Fetching "$FILE"
  mkdir -p "tendermint/$(dirname $FILE)"
  curl -sSL "$TM_PROTO_URL/$FILE" > "tendermint/$FILE"
done
