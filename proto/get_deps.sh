#!/usr/bin/env bash

cd "$(dirname "${BASH_SOURCE[0]}")"

CM_VERSION="v0.37.1"
CM_PROTO_URL=https://raw.githubusercontent.com/rollkit/cometbft/$CM_VERSION/proto/tendermint

CM_PROTO_FILES=(
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

echo Fetching protobuf dependencies from CommetBFT $CM_VERSION
for FILE in "${CM_PROTO_FILES[@]}"; do
  echo Fetching "$FILE"
  mkdir -p "tendermint/$(dirname $FILE)"
  curl -sSL "$CM_PROTO_URL/$FILE" > "tendermint/$FILE"
done
