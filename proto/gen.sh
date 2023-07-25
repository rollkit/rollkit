#!/usr/bin/env bash

# see: https://stackoverflow.com/a/246128
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"/..
TARGET_DIR=./types/pb

cd $SCRIPT_DIR
mkdir -p $TARGET_DIR
rm -rf $TARGET_DIR/*
docker run -v $PWD:/workspace --workdir /workspace tendermintdev/docker-build-proto sh ./proto/protoc.sh

cp -r ./proto/pb/* $TARGET_DIR/

rm -rf $TARGET_DIR/tendermint
