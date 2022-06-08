#!/usr/bin/env bash

# see: https://stackoverflow.com/a/246128
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
cd $SCRIPT_DIR

TM_VERSION=v0.34.14
TM_PROTO_URL=https://raw.githubusercontent.com/tendermint/tendermint/$TM_VERSION/proto/tendermint
GOGO_PROTO_URL=https://raw.githubusercontent.com/regen-network/protobuf/cosmos

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
for FILE in "${TM_PROTO_FILES[@]}";
do
  echo Fetching $FILE
  mkdir -p tendermint/`dirname $FILE`
  curl -sSL "$TM_PROTO_URL/$FILE" > "tendermint/$FILE"
  #GO_PACKAGE="github.com/celestiaorg/optimint/types/pb/tendermint/`dirname $FILE`"
  #echo Fixing go_package to $GO_PACKAGE
  #sed -i -e 's|option go_package = ".*";|option go_package = "'"$GO_PACKAGE"'";|' "tendermint/$FILE"
done
