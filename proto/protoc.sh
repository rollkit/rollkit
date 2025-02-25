#!/usr/bin/env bash

set -eo pipefail

buf generate --path="./proto/rollkit" --template="buf.gen.yaml" --config="buf.yaml"
