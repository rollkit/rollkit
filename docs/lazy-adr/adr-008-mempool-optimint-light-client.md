# ADR 008: Light Client Transaction Gossip & Mempool

## Changelog

- 20.09.2022: Initial Draft
- 29.09.2022: Rename Optimint to rollmint
- 22.01.2023: Rename rollmint to Rollkit

## Context

Rollkit Light Clients cannot validate transactions without a state. Therefore Light Clients should not gossip incoming transactions, and the mempool can be disabled.

### Explanation

There is a wish for the light clients to participate in the P2P Layer. One of the ways a full node participates in the network is to gossip valid transactions throughout the network. Each full node that receives a transaction sends an ABCI message, `CheckTx`, to the application layer to check for validity and receives an `abci.ResponseCheckTx` .
There are 2 Types of checks.
Current stateless checks:

- Check that the size is less than the configured maximum transaction size.
- Call any Pre-Check hooks if defined
- Check if the proxy connection has an error
- Check if the transaction is already in cache / mempool

Stateful:

- Checks if transactions and messages are valid based on a committed state

Light clients cannot do stateful checks because they don't have access to the state.
Light clients can do stateless checks. However, creating invalid transactions that pass the current stateless checks is easy. Light clients could therefore support a DOS attack of the network when they gossip invalid transactions.
If light clients do not check transactions, they do not need the mempool.

### Libp2p pubsub

If the transaction originates from the light client i.e. submitting a new transaction, then this transaction must be gossiped to the network.

A light client will use the [fan-out](https://docs.libp2p.io/concepts/publish-subscribe/#fan-out) functionality of pubsub. It will send its transaction to the network but will not subscribe to receiving and propagating other transactions.

## Alternative Approaches

- We create more rigorous stateless checks on the transactions that would reduce or prevent the DOS attack and enable transaction gossiping

## Status

Proposed

## Consequences

### Positive

- Reduction of complexity and keeping the light client lightweight

### Negative

- Light clients do not participate in gossiping transactions

## References

Issue #100 [References](https://github.com/celestiaorg/oollmint/issues/100#issuecomment-921848268)
