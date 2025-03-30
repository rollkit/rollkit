# go-da

go-da defines a generic Data Availability interface for modular blockchains.

<!-- markdownlint-disable MD013 -->
[![build-and-test](https://github.com/rollkit/go-da/actions/workflows/ci_release.yml/badge.svg)](https://github.com/rollkit/go-da/actions/workflows/ci_release.yml)
[![golangci-lint](https://github.com/rollkit/go-da/actions/workflows/lint.yml/badge.svg)](https://github.com/rollkit/go-da/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rollkit/go-da)](https://goreportcard.com/report/github.com/rollkit/go-da)
[![codecov](https://codecov.io/gh/rollkit/go-da/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/rollkit/go-da)
[![GoDoc](https://godoc.org/github.com/rollkit/go-da?status.svg)](https://godoc.org/github.com/rollkit/go-da)
<!-- markdownlint-enable MD013 -->

## DA Interface

| Method        | Params                                                   | Return          |
| ------------- | -------------------------------------------------------- | --------------- |
| `MaxBlobSize` |                                                          | `uint64`        |
| `Get`         | `ids []ID, namespace Namespace`                          | `[]Blobs`       |
| `GetIDs`      | `height uint64, namespace Namespace`                     | `[]ID`          |
| `GetProofs`      | `ids []id, namespace Namespace`                     | `[]Proof`          |
| `Commit`      | `blobs []Blob, namespace Namespace`                      | `[]Commitment`  |
| `Validate`    | `ids []Blob, proofs []Proof, namespace Namespace`        | `[]bool`        |
| `Submit`      | `blobs []Blob, gasPrice float64, namespace Namespace`    | `[]ID` |

NOTE: The `Namespace` parameter in the interface methods is optional and used
only on DA layers that support the functionality, for example Celestia
namespaces and Avail AppIDs.

## Implementations

The following implementations are available:

* [celestia-da](https://github.com/rollkit/celestia-da) implements Celestia as DA.
* [avail-da](https://github.com/rollkit/avail-da) implements Polygon Avail as DA.

In addition the following helper implementations are available:

* [DummyDA](https://github.com/rollkit/go-da/blob/main/test/dummy.go) implements
a Mock DA useful for testing.
* [Proxy](https://github.com/rollkit/go-da/tree/main/proxy) implements a proxy
server that forwards requests to a gRPC server. The proxy client
can be used directly to interact with the DA service.

## Helpful commands

```sh
# Generate protobuf files. Requires docker.
make proto-gen

# Lint protobuf files. Requires docker.
make proto-lint

# Run tests.
make test

# Run linters (requires golangci-lint, markdownlint, hadolint, and yamllint)
make lint
```

## Contributing

We welcome your contributions! Everyone is welcome to contribute, whether it's
in the form of code, documentation, bug reports, feature
requests, or anything else.

If you're looking for issues to work on, try looking at the
[good first issue list](https://github.com/rollkit/go-da/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).
Issues with this tag are suitable for a new external contributor and is a great
way to find something you can help with!

Please join our
[Community Discord](https://discord.com/invite/YsnTPcSfWQ)
to ask questions, discuss your ideas, and connect with other contributors.

## Code of Conduct

See our Code of Conduct [here](https://docs.celestia.org/community/coc).
