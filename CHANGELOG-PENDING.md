# Unreleased Changes

## vX.Y.Z

Month, DD, YYYY

### BREAKING CHANGES

- [go package] (Link to PR) Description @username

### FEATURES

- [indexer] [Implement block and transaction indexing, enable TxSearch RPC endpoint #202](https://github.com/celestiaorg/optimint/pull/202) [@mattdf](https://github.com/mattdf)
- [rpc] [Tendermint URI RPC #224](https://github.com/celestiaorg/optimint/pull/224) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Subscription methods #252](https://github.com/celestiaorg/optimint/pull/252) [@tzdybal](https://github.com/tzdybal/)

### IMPROVEMENTS

- [ci] [Add more linters #219](https://github.com/celestiaorg/optimint/pull/219) [@tzdybal](https://github.com/tzdybal/)
- [deps] [Update dependencies: grpc, cors, cobra, viper, tm-db #245](https://github.com/celestiaorg/optimint/pull/245) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement NumUnconfirmedTxs #255](https://github.com/celestiaorg/optimint/pull/255) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement BlockByHash #256](https://github.com/celestiaorg/optimint/pull/256) [@mauriceLC92](https://github.com/mauriceLC92)
- [rpc] [Implement BlockResults #263](https://github.com/celestiaorg/optimint/pull/263) [@tzdybal](https://github.com/tzdybal/)
- [store,indexer] [Replace tm-db dependency with store package #268](https://github.com/celestiaorg/optimint/pull/268) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement ConsensusState/DumpConsensusState #273](https://github.com/celestiaorg/optimint/pull/273) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement Tx Method #272](https://github.com/celestiaorg/optimint/pull/272) [@mauriceLC92](https://github.com/mauriceLC92)
- [rpc] [Implement Commit and BlockSearch method #258](https://github.com/celestiaorg/optimint/pull/258) [@raneet10](https://github.com/Raneet10/)
- [rpc] [Remove extra variable #280](https://github.com/celestiaorg/optimint/pull/280) [@raneet10](https://github.com/Raneet10/)
- [rpc] [Implement BlockChainInfo RPC method #282](https://github.com/celestiaorg/optimint/pull/282) [@raneet10](https://github.com/Raneet10/)
- [state,block,store,rpc] [Minimalistic validator set handling](https://github.com/celestiaorg/optimint/pull/286) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement ConsensusParams #292](https://github.com/celestiaorg/optimint/pull/292) [@tzdybal](https://github.com/tzdybal/)
- [rpc] [Implement GenesisChunked method #287](https://github.com/celestiaorg/optimint/pull/287) [@mauriceLC92](https://github.com/mauriceLC92)

### BUG FIXES

- [store] [Use KeyCopy instead of Key in BadgerIterator #274](https://github.com/celestiaorg/optimint/pull/274) [@tzdybal](https://github.com/tzdybal/)
- [state,block] [Do save ABCI responses for blocks #285](https://github.com/celestiaorg/optimint/pull/285) [@tzdybal](https://github.com/tzdybal/)
- [conv/abci] [Map LastBlockID.Hash to LastHeaderHash in conversion #303](https://github.com/celestiaorg/optimint/pull/303) [@tzdybal](https://github.com/tzdybal/)
- [chore] [Fix multiple bugs for Ethermint #305](https://github.com/celestiaorg/optimint/pull/305) [@tzdybal](https://github.com/tzdybal/)

- [go package] (Link to PR) Description @username
