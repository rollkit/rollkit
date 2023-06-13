# Rollkit

A modular framework for rollups, with an ABCI-compatible client interface. For more in-depth information about Rollkit, please visit our [website](https://rollkit.dev).

[![build-and-test](https://github.com/rollkit/rollkit/actions/workflows/test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/test.yml)
[![golangci-lint](https://github.com/rollkit/rollkit/actions/workflows/lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rollkit/rollkit)](https://goreportcard.com/report/github.com/rollkit/rollkit)
[![codecov](https://codecov.io/gh/rollkit/rollkit/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/rollkit/rollkit)
[![GoDoc](https://godoc.org/github.com/rollkit/rollkit?status.svg)](https://godoc.org/github.com/rollkit/rollkit)

## Building From Source

Requires Go version >= 1.19.

To build:

```sh
git clone https://github.com/rollkit/rollkit.git
cd rollkit 
go build -v ./...
```

## Building With Rollkit

While Rollkit is a modular framework that aims to be compatible with a wide
range of data availability layers, settlement layers, and execution
environments, the most supported development environment is building on Celestia
as a data availability layer.

### Building On Celestia

There are currently 2 ways to build on Celestia:

1. Using a local development environment with [local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet)
1. Using the Arabica or Mocha Celestia testnet

#### Local Development Environment

The current Rollkit v0.8.1 release is compatible with the
[local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet)
v0.10.4 release. This version combination is compatible with
[celesita-app](https://github.com/celestiaorg/celestia-app) v0.13.3 and
[celestia-node](https://github.com/celestiaorg/celestia-node) v0.10.4.

For more information see the [release
notes](https://github.com/rollkit/local-celestia-devnet/releases/tag/v0.10.4)
for v0.10.4 of local-celestia-devnet.

#### Arabica and Mocha Testnets

> :warning: **Rollkit v0.8.1 is not compatible with latest releases of Arabica and Mocha.** :warning:

Arabica compatibility is currently under development on the `arabica` branch of
the rollkit repo. If you are need to test or build on Arabica, please reach out
the Rollkit team and track the progress of the `arabica` branch and the
v0.8.2-rcx releases.

The key updates to making Rollkit compatible with Arabica and Mocha are making
Rollkit compatible with celestia-app v1.0.0-rcX and celestia-node v0.11.0-rcX.

### Tools

1. Install [golangci-lint](https://golangci-lint.run/usage/install/)
1. Install [markdownlint](https://github.com/DavidAnson/markdownlint)
1. Install [hadolint](https://github.com/hadolint/hadolint)
1. Install [yamllint](https://yamllint.readthedocs.io/en/stable/quickstart.html)

## Helpful Commands

```sh
# Run unit tests
make test-unit

# Run unit tests with the data race detector
make test-unit-race

# Run tests with and without the data race detector
make test-all

# Generate protobuf files (requires Docker)
make proto-gen

# Run linters (requires golangci-lint, markdownlint, hadolint, and yamllint)
make lint

# Lint protobuf files (requires Docker and buf)
make proto-lint

```

## Contributing

We welcome your contributions! Everyone is welcome to contribute, whether it's in the form of code,
documentation, bug reports, feature requests, or anything else.

If you're looking for issues to work on, try looking at the [good first issue list](https://github.com/rollkit/rollkit/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22). Issues with this tag are suitable for a new external contributor and is a great way to find something you can help with!

See [the contributing guide](./CONTRIBUTING.md) for more details.

Please join our [Community Discord](https://discord.com/invite/YsnTPcSfWQ) to ask questions, discuss your ideas, and connect with other contributors.

## Dependency Graph

To see our progress and a possible future of Rollkit visit our [Dependency Graph](./docs/specification/rollkit-dependency-graph.md).

## Code of Conduct

See our Code of Conduct [here](https://docs.celestia.org/community/coc).
