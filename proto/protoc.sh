#!/usr/bin/env bash

set -eo pipefail

buf generate --path="./proto/dalc" --template="buf.gen.yaml" --config="buf.yaml"
buf generate --path="./proto/rollkit" --template="buf.gen.yaml" --config="buf.yaml"
buf generate --path="./proto/tendermint/abci" --template="buf.gen.yaml" --config="buf.yaml"
