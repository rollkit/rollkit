#!/bin/bash

DOCKER_TAG=$1
arch=$(uname -p)

WORKDIR=$PWD

cd .. 

# Setup Ethermint
cp -R optimint ethermint
cd ethermint
rm -rf .git
go mod edit -replace=github.com/celestiaorg/optimint=./optimint
go mod tidy -compat=1.17 -e

cd ..

# Setup Evmos
cp -R ethermint evmos
cd evmos
go mod edit -replace=github.com/tharsis/ethermint=./ethermint
go mod tidy -compat=1.17 -e

# Log the go.mod for debug
cat go.mod 

# Docker build
if [ $arch=x86_64 ]
then
    docker buildx build --platform linux/amd64 -f docker/Dockerfile -t $DOCKER_TAG .
elif [ $arch=arm ] 
then
    docker buildx build --platform linux/arm64 -f docker/Dockerfile -t $DOCKER_TAG .
else
    echo "architecture is not one of x86_64 or arm"
fi
