# Mempool

## Abstract

The mempool module provides an interface to check the validity of incoming transactions, and store transactions which have not yet been included in a block. It's defined by an interface [here](https://github.com/rollkit/rollkit/blob/main/mempool/mempool.go#L16), with an implementation [here](https://github.com/rollkit/rollkit/blob/main/mempool/v1/mempool.go).

## Component Description

Full nodes instantiate a mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L133). A `p2p.GossipValidator` is constructed from the node's mempool [here](https://github.com/rollkit/rollkit/blob/main/node/full.go#L391), which is used by Rollkit's p2p code to deal with peers who gossip invalid transactions.