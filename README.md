# Rollkit

A modular framework for rollups, with an ABCI-compatible client interface. For more in-depth information about Rollkit, please visit our [website](https://rollkit.dev).

<!-- markdownlint-disable MD013 -->
[![build-and-test](https://github.com/rollkit/rollkit/actions/workflows/test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/test.yml)  
[![golangci-lint](https://github.com/rollkit/rollkit/actions/workflows/lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/lint.yml)  
[![Go Report Card](https://goreportcard.com/badge/github.com/rollkit/rollkit)](https://goreportcard.com/report/github.com/rollkit/rollkit)  
[![codecov](https://codecov.io/gh/rollkit/rollkit/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/rollkit/rollkit)  
[![GoDoc](https://godoc.org/github.com/rollkit/rollkit?status.svg)](https://godoc.org/github.com/rollkit/rollkit)
<!-- markdownlint-enable MD013 -->

## Building from source

Requires Go version >= 1.21.

To build:

```sh
git clone https://github.com/rollkit/rollkit.git
cd rollkit
go build -v ./...
```

## Building with Rollkit

While Rollkit is a modular framework that aims to be compatible with a wide
range of data availability layers, settlement layers, and execution
environments, the most supported development environment is building on Celestia
as a data availability layer.

## Rollkit CLI

A cli tool that allows you to run different kinds of nodes for a rollkit network while also helping you generate the required configuration files

### Install

To install `rollkit`, simply run the following command at the root of the rollkit repo

```bash
make install
```

The latest Rollkit is now installed. You can verify the installation by running:

```bash
rollkit version
```

Explore the CLI documentation [here](./cmd/rollkit/docs/rollkit.md)

### Building on Celestia

There are currently 2 ways to build on Celestia:

1. Using a local development environment with [local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet)
1. Using the Arabica or Mocha Celestia testnet

#### Compatibility

| network               | rollkit | celestia-node | celestia-app |
| --------------------- | ------- | ------------- | ------------ |
| local-celestia-devnet | v0.11.9 | v0.12.2-rc1   | v1.4.0       |
| arabica               | v0.11.9 | v0.12.1       | v1.6.0       |

<!-- markdownlint-disable MD013 -->
| rollkit/cosmos-sdk | rollkit/cometbft | rollkit |
|-|-|-|
| [v0.50.1-rollkit-v0.11.9-no-fraud-proofs](https://github.com/rollkit/cosmos-sdk/releases/tag/v0.50.1-rollkit-v0.11.9-no-fraud-proofs) | v0.38.0-rc3| [v0.11.9](https://github.com/rollkit/rollkit/releases/tag/v0.11.9) |
<!-- markdownlint-enable MD013 -->

#### Local development environment

The Rollkit v0.11.9 release is compatible with the
[local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet) [v0.12.5](https://github.com/rollkit/local-celestia-devnet/releases/tag/v0.12.5)
release. This version combination is compatible with celestia-app
[v1.4.0](https://github.com/celestiaorg/celestia-app/releases/tag/v1.4.0)
and celestia-node
[v0.12.2-rc1](https://github.com/celestiaorg/celestia-node/releases/tag/v0.12.2-rc1).

#### Arabica devnet and Mocha testnet

The Rollkit v0.11.9 release is compatible with
[arabica-10](https://docs.celestia.org/nodes/arabica-devnet/) devnet
[mocha-4](https://docs.celestia.org/nodes/mocha-testnet/) testnet which are running
celestia-app
[v1.6.0](https://github.com/celestiaorg/celestia-app/releases/tag/v1.6.0)
and celestia-node
[v0.12.1](https://github.com/celestiaorg/celestia-node/releases/tag/v0.12.1).

#### Cometbft v0.37.x and Cosmos-SDK v0.47.x

The Rollkit v0.10.7 release is compatible with Cometbft v0.37.2 and Cosmos-SDK
v0.47.6. However, this version is no longer supported for future developements and
it is recommended to Rollkit v0.11.x.

### Tools

1. Install [golangci-lint](https://golangci-lint.run/usage/install/)
1. Install [markdownlint](https://github.com/DavidAnson/markdownlint)
1. Install [hadolint](https://github.com/hadolint/hadolint)
1. Install [yamllint](https://yamllint.readthedocs.io/en/stable/quickstart.html)

## Helpful commands

```sh
# Run unit tests
make test

# Generate protobuf files (requires Docker)
make proto-gen

# Run linters (requires golangci-lint, markdownlint, hadolint, and yamllint)
make lint

# Lint protobuf files (requires Docker and buf)
make proto-lint

```

## Contributing

We welcome your contributions! Everyone is welcome to contribute, whether it's
in the form of code, documentation, bug reports, feature
requests, or anything else.

If you're looking for issues to work on, try looking at the
[good first issue list](https://github.com/rollkit/rollkit/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).
Issues with this tag are suitable for a new external contributor and is a great
way to find something you can help with!

See
[the contributing guide](https://github.com/rollkit/rollkit/blob/main/CONTRIBUTING.md)
for more details.

Please join our
[Community Discord](https://discord.com/invite/YsnTPcSfWQ)
to ask questions, discuss your ideas, and connect with other contributors.

## Dependency graph

To see our progress and a possible future of Rollkit visit our [Dependency
Graph](https://github.com/rollkit/rollkit/blob/main/specs/src/specs/rollkit-dependency-graph.md).

## Code of Conduct

See our Code of Conduct [here](https://docs.celestia.org/community/coc).
