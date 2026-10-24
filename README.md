# EV-node

Ev-node is the basis of the Evolve Stack. For more in-depth information about Evolve, please visit our [website][docs].

[![Go Report Card](https://goreportcard.com/badge/github.com/evstack/ev-node)](https://goreportcard.com/report/github.com/evstack/ev-node)
[![codecov](https://codecov.io/gh/evstack/ev-node/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/evstack/ev-node)
[![GoDoc](https://godoc.org/github.com/evstack/ev-node?status.svg)](https://godoc.org/github.com/evstack/ev-node)

> **⚠️ Version Notice**: Do not use tags or releases before v1.*. Pre-v1 releases are not stable and should be considered abandoned.

## Using Evolve

Evolve supports multiple sync modes:

- **Hybrid sync**: Sync from both DA layer and P2P network (default when peers are configured)
- **DA-only sync**: Sync exclusively from DA layer by leaving P2P peers empty (see [Configuration Guide](docs/learn/config.md#da-only-sync-mode))
- **P2P-priority sync**: Prioritize P2P with DA as fallback

### Example Implementation: TestApp CLI

The easiest way to understand how to use Evolve is by exploring our example implementation, TestApp.

Requires Go version >= 1.26.0, as specified in [`go.mod`](go.mod).

TestApp is a CLI tool that demonstrates how to run different kinds of nodes using the Evolve framework.
It serves as a reference implementation and helps you understand how to build your own Evolve-based blockchain.

#### Install

To install the example `testapp`, simply run the following command at the root of the
ev-node repo:

```bash
just install
```

The latest TestApp example is now installed. You can verify the installation by running:

```bash
testapp version
```

#### Quick Start

You can spin up a local TestApp network (powered by Evolve) with the following command:

```bash
testapp start
```

## Building with Evolve

Evolve is the first sovereign application framework that allows you to launch
a sovereign, customizable blockchain as easily as a smart contract.
TestApp serves as a reference implementation to help you get started with your own Evolve-based blockchain.

Check out our tutorials on our [website][docs].

## Onboarding with Claude Code

Use [Claude Code](https://code.claude.com/docs/en/overview) to explore the codebase:

```bash
cd ev-node && claude
```

Example prompts:

| Goal | Prompt |
|------|--------|
| Overview | "How does ev-node work?" |
| Block package | "Explain block production flow" |
| DA layer | "How does the DA layer work?" |
| Sequencing | "Explain single vs based sequencer" |

The `.claude/skills/ev-node-explainer/` skill provides architecture docs for block, DA, and sequencing systems.

## Contributing

We welcome your contributions! Everyone is welcome to contribute, whether it's
in the form of code, documentation, bug reports, feature
requests, or anything else.

If you're looking for issues to work on, try looking at the
[good first issue list](https://github.com/evstack/ev-node/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22).
Issues with this tag are suitable for a new external contributor and is a great
way to find something you can help with!

See
[the contributing guide](https://github.com/evstack/ev-node/blob/main/CONTRIBUTING.md)
for more details.

Please join our
[Community Discord](https://discord.com/invite/YsnTPcSfWQ)
to ask questions, discuss your ideas, and connect with other contributors.

### Helpful commands

```sh
# Run unit tests
just test

# Generate protobuf files (requires Docker)
just proto-gen

# Run linters (requires golangci-lint, markdownlint, hadolint, and yamllint)
just lint

# Lint protobuf files (requires Docker and buf)
just proto-lint

```

### Tools

1. Install [golangci-lint](https://golangci-lint.run/welcome/install/)
1. Install [markdownlint](https://github.com/DavidAnson/markdownlint)
1. Install [hadolint](https://github.com/hadolint/hadolint)
1. Install [yamllint](https://yamllint.readthedocs.io/en/stable/quickstart.html)

## Audits

| Date | Auditor | Version | Report |
|---|---|---|---|
| 2024/01/12 | [Informal Systems](https://informal.systems/) | [eccdd...bcb9d](https://github.com/evstack/ev-node/commit/eccdd0f1793a5ac532011ef4d896de9e0d8bcb9d) | [informal-systems.pdf](docs/audit/informal-systems.pdf) |
| 2024/01/10 | [Binary Builders](https://binary.builders/)   | [eccdd...bcb9d](https://github.com/evstack/ev-node/commit/eccdd0f1793a5ac532011ef4d896de9e0d8bcb9d) | [binary-builders.pdf](docs/audit/binary-builders.pdf)   |

[docs]: <https://docs.ev.xyz>
