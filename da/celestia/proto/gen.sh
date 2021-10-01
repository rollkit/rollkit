#!/usr/bin/env bash

# see: https://stackoverflow.com/a/246128
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" &> /dev/null && pwd )"

cd $SCRIPT_DIR

protoc -I=. -I=$GOPATH/src -I=$GOPATH/src/github.com/gogo/protobuf\
	--gofast_out=plugins=grpc:../pb\
	celestia.proto 
