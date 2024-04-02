# Rollkit

A modular framework for rollups, with an ABCI-compatible client interface. For more in-depth information about Rollkit, please visit our [website](https://rollkit.dev).

<!-- markdownlint-disable MD013 -->
[![build-and-test](https://github.com/rollkit/rollkit/actions/workflows/test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/test.yml)
[![golangci-lint](https://github.com/rollkit/rollkit/actions/workflows/lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rollkit/rollkit)](https://goreportcard.com/report/github.com/rollkit/rollkit)
[![codecov](https://codecov.io/gh/rollkit/rollkit/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/rollkit/rollkit)
[![GoDoc](https://godoc.org/github.com/rollkit/rollkit?status.svg)](https://godoc.org/github.com/rollkit/rollkit)
<!-- markdownlint-enable MD013 -->

## Rollkit CLI

Requires Go version >= 1.21.

A cli tool that allows you to run different kinds of nodes for a rollkit network
while also helping you generate the required configuration files

### Install

To install `rollkit`, simply run the following command at the root of the
rollkit repo

```bash
make install
```

The latest Rollkit is now installed. You can verify the installation by running:

```bash
rollkit version
```

Explore the CLI documentation [here](./cmd/rollkit/docs/rollkit.md)

## Building with Rollkit

While Rollkit is a modular framework that aims to be compatible with a wide
range of data availability layers, settlement layers, and execution
environments, the most supported development environment is building on Celestia
as a data availability layer.

### Building on Celestia

You can build Rollkit rollups with [local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet),
[arabica devnet](https://docs.celestia.org/nodes/arabica-devnet) and
[mocha testnet](https://docs.celestia.org/nodes/mocha-testnet) and
[mainnet beta](https://docs.celestia.org/nodes/mainnet).

#### Compatibility

| network               | rollkit | celestia-node | celestia-app |
| --------------------- | ------- | ------------- | ------------ |
| local-celestia-devnet | v0.13.0 | v0.13.1       | v1.7.0       |
| arabica               | v0.13.0 | v0.13.1       | v1.7.0       |
| mocha                 | v0.13.0 | v0.13.1       | v1.7.0       |
| mainnet               | v0.13.0 | v0.13.1       | v1.7.0       |

<!-- markdownlint-disable MD013 -->
| rollkit/cosmos-sdk | rollkit/cometbft | rollkit |
|-|-|-|
| [v0.50.5-rollkit-v0.13.0-no-fraud-proofs](https://github.com/rollkit/cosmos-sdk/releases/tag/v0.50.5-rollkit-v0.13.0-no-fraud-proofs) | v0.38.5| [v0.13.0](https://github.com/rollkit/rollkit/releases/tag/v0.13.0) |
<!-- markdownlint-enable MD013 -->

#### Local development environment

The Rollkit v0.13.0 release is compatible with the
[local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet) [v0.13.1](https://github.com/rollkit/local-celestia-devnet/releases/tag/v0.13.1)
release. This version combination is compatible with celestia-app
[v1.7.0](https://github.com/celestiaorg/celestia-app/releases/tag/v1.7.0)
and celestia-node
[v0.13.1](https://github.com/celestiaorg/celestia-node/releases/tag/v0.13.1).

#### Arabica devnet and Mocha testnet

The Rollkit v0.13.0 release is compatible with
[arabica-11](https://docs.celestia.org/nodes/arabica-devnet/) devnet
[mocha-4](https://docs.celestia.org/nodes/mocha-testnet/) testnet which are running
celestia-app
[v1.7.0](https://github.com/celestiaorg/celestia-app/releases/tag/v1.7.0)
and celestia-node
[v0.13.1](https://github.com/celestiaorg/celestia-node/releases/tag/v0.13.1).

#### Celestia mainnet

The Rollkit v0.13.0 release is compatible with [mainnet-beta](https://docs.celestia.org/nodes/mainnet/)
which is running celestia-app
[v1.7.0](https://github.com/celestiaorg/celestia-app/releases/tag/v1.7.0)
and celestia-node
[v0.13.1](https://github.com/celestiaorg/celestia-node/releases/tag/v0.13.1).

#### Cometbft v0.37.x and Cosmos-SDK v0.47.x

The Rollkit v0.10.7 release is compatible with Cometbft v0.37.2 and Cosmos-SDK
v0.47.6. However, this version is no longer supported for future developments and
it is recommended to use Rollkit v0.13.x.

### Tools

1. Install [golangci-lint](https://golangci-lint.run/welcome/install/)
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

## Audits

| Date       | Auditor                                       | Version                                                                             | Report                                                  |
|------------|-----------------------------------------------|-------------------------------------------------------------------------------------|---------------------------------------------------------|
| 2024/01/12  | [Informal Systems](https://informal.systems/) | [eccdd...bcb9d](https://github.com/rollkit/rollkit/commit/eccdd0f1793a5ac532011ef4d896de9e0d8bcb9d)   | [informal-systems.pdf](specs/audit/informal-systems.pdf) |
| 2024/01/10 | [Binary Builders](https://binary.builders/)   | [eccdd...bcb9d](https://github.com/rollkit/rollkit/commit/eccdd0f1793a5ac532011ef4d896de9e0d8bcb9d) | [binary-builders.pdf](specs/audit/binary-builders.pdf)   |

