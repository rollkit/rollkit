#!/usr/bin/env bash

# see: https://stackoverflow.com/a/246128
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"
TARGET_DIR=../types/pb
IMPORTS="-I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf"

cd $SCRIPT_DIR
rm -rf $TARGET_DIR/*
protoc $IMPORTS --gofast_out=paths=source_relative:. optimint/optimint.proto
protoc $IMPORTS --gofast_out=plugins=grpc,paths=source_relative:. dalc/dalc.proto
mv github.com/celestiaorg/optimint/types/pb/* $TARGET_DIR/
rm -rf github.com
