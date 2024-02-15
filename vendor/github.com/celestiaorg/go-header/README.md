# Go Header

[![Go Reference](https://pkg.go.dev/badge/github.com/celestiaorg/go-header.svg)](https://pkg.go.dev/github.com/celestiaorg/go-header)
[![GitHub release (latest by date including pre-releases)](https://img.shields.io/github/v/release/celestiaorg/go-header)](https://github.com/celestiaorg/go-header/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/celestiaorg/go-header)](https://goreportcard.com/report/github.com/celestiaorg/go-header)
[![codecov](https://codecov.io/gh/celestiaorg/go-header/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/celestiaorg/go-header)

Go Header contains all services related to generating, requesting, syncing and storing Headers.

There are 4 main components in the header package:

 1. p2p.Subscriber listens for new Headers from the P2P network (via the
    HeaderSub)
 2. p2p.Exchange request Headers from other nodes
 3. Syncer manages syncing of historical and new Headers from the P2P network
 4. Store manages storing Headers and making them available for access by other
    dependent services.

## Table of Contents

- [Go Header](#go-header)
- [Table of Contents](#table-of-contents)
- [Minimum requirements](#minimum-requirements)
- [Installation](#installation)
- [Package-specific documentation](#package-specific-documentation)
- [Code of Conduct](#code-of-conduct)

## Minimum requirements

| Requirement | Notes          |
| ----------- | -------------- |
| Go version  | 1.20 or higher |

## Installation

```sh
go get -u https://github.com/celestiaorg/go-header.git
```

For more information, go visit our docs at <https://pkg.go.dev/github.com/celestiaorg/go-header>.

## Package-specific documentation

- [P2P](./p2p/doc.go)
- [Sync](./sync/doc.go)
- [Store](./store/doc.go)

## Code of Conduct

See our Code of Conduct [here](https://docs.celestia.org/community/coc).
