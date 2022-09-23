# ADR 008: Light Client Transaction Gossip & Mempool

## Changelog

- 20.09.2022: Initial Draft

## Context

Optimint Light Clients cannot validate transactions without state. They should therfore not gossip incomming transactions and the mempool can be disabled.  

### Explanation

There is wish for the light clients to participate in the P2P Layer. One of the ways a full node participates in the network is to gossip valid transactions throuout the network. Each full-node that receives a transaction sends an ABCI message , ```CheckTx```, to the application layer to check for validity, and receives an ```abci.ResponseCheckTx```.
There are 2 Types of checks.
Current stateless checks:

- Check that the size is less then the configured maximum transaction size.
- Call any Pre-Check hooks if defined
- Check if proxy connection has an error
- Check if transaction is already in cache / mempool

Statefull:

- checks if transaction and messages are valid based on a committed state

Light clients cannot do statefull checks, because they don't have access to the state.
Light clients can do stateless checks. Although it is easy to create invalid transactions that pass the current stateless checks. Light clients could therefore support a DOS-attack of the newtwork when they gossip invalid transactions.
If light clients do not check transactions then they do not need the mempool.

### Libp2p pubsub

If the transaction orginates from the light client i.e. submitting a new transaction, then this trancastion must be gossiped to the network.

A light client will use the [fan-out](https://docs.libp2p.io/concepts/publish-subscribe/#fan-out) functionality of pubsub. It will send out its transaction to the network but will not subsribe to receiving and propergating other transactions.

## Alternative Approaches

- We create more rigourous stateless checks on the transactions that would reduce or prevent the DOS-attack and enable transaction gossiping

## Status

Proposed

## Consequences

### Positive

- Reduction of complexity and keeping the light client lightweight

### Negative

- Light clients do not participate in gossiping transactions

## References

Issue #100 [Refereces](https://github.com/celestiaorg/optimint/issues/100#issuecomment-921848268)
