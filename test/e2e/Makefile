#!/usr/bin/make -f

BUILDDIR ?= $(CURDIR)/build
INTEGRATION_TEST_DIR:=$(shell dirname $(realpath $(firstword $(MAKEFILE_LIST))))
OPTIMINT_DIR:=$(shell dirname $(shell dirname $(INTEGRATION_TEST_DIR)))

# rollup builds a binary of the rollup that has an extra 'optimint-start' command
rollup: $(BUILDDIR)/
	go mod edit -replace github.com/celestiaorg/optimint=$(OPTIMINT_DIR)
	go build -o $(BUILDDIR)/rollupd $(INTEGRATION_TEST_DIR)/rollupd/
.PHONY: rollup

# install-rollup installs the binary of a rollup has an extra 'optimint-start' command
install-rollup:
	cd ./rollupd && go install
.PHONY: install-rollup

init-rollup:
	sh docker/optimint/init.sh

# builds a docker image of the rollup that uses optimint
rollup-docker:
	docker build --tag optimint/e2e-node -f docker/optimint/Dockerfile ../..
.PHONY: build-docker

# mock-dalc builds a binary that will run the mock server
mock-dalc: $(BUILDDIR)/
	go mod edit -replace github.com/celestiaorg/optimint=$(OPTIMINT_DIR)
	go build -o $(BUILDDIR)/dalc $(INTEGRATION_TEST_DIR)/dalc/
.PHONY: mock-dalc

# builds a docker image of the rollup that uses optimint
mock-dalc-docker:
	docker build --tag dalc/mock -f docker/mockdalc/Dockerfile ../..
.PHONY: build-docker
