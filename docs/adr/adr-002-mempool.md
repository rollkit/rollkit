# Mempool

For now, mempool implementation from lazyledger-core/Tendermint will be used.

## Pros

* good integration with other re-used code (see ADR-001)
* well tested
* glue code is not required
* it will be updated in case of ABCI++ adoption
* mempool doesn't depend on P2P layer, so it's easy to replace it with libp2p
* mempool does not require any knowledge about the internal structure of the Txs and is already "abci-ready"

## Cons

* inherit all limitations of the tendermint mempool
  * no prioritization of Txs
  * many [open issues](https://github.com/cometbft/cometbft/issues?q=is%3Aissue+is%3Aopen+mempool+label%3AC%3Amempool)
* legacy code base (the tendermint mempool exists for a while now)

## Alternatives

* Implementation from scratch
  * time consuming
  * error prone
* Re-using other mempool (Celo, Prysm, etc)
  * different API
  * potential licensing issues
