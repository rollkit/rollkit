# Rollkit CLI

A cli tool that allows you to run different kinds of nodes for a rollkit network while also helping you generate the required configuration files

## Install

NOTE: Requires Go version >= 1.19.

To install `rollkit`, simply run the following command at the root of the rollkit repo

```bash
go install ./cmd/rollkit
```

The latest Rollkit is now installed. You can verify the installation by running:

```bash
rollkit version
```

## Reinstall

If you have Rollkit installed, and you make updates that you want to test, simply run:

```bash
go install ./cmd/rollkit
```
