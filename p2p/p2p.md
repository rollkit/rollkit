# P2P

Every rollup node (both full and light) runs a P2P client using [go-libp2p][go-libp2p] P2P networking stack for gossiping transactions in the rollup's P2P network. The same P2P client is also used by the header and block sync services for gossiping headers and blocks.

Following parameters are required for creating a new instance of a P2P client:

* P2PConfig (described below)
* [go-libp2p][go-libp2p] private key used to create a libp2p connection and join the p2p network.
* chainID: rollup identifier used as namespace within the p2p network for peer discovery. The namespace acts as a sub network in the p2p network, where peer connections are limited to the same namespace.
* datastore: an instance of [go-datastore][go-datastore] used for creating a connection gator and stores blocked and allowed peers.
* logger

```go
// P2PConfig stores configuration related to peer-to-peer networking.
type P2PConfig struct {
	ListenAddress string // Address to listen for incoming connections
	Seeds         string // Comma separated list of seed nodes to connect to
	BlockedPeers  string // Comma separated list of nodes to ignore
	AllowedPeers  string // Comma separated list of nodes to whitelist
}
```

A P2P client also instantiates a [connection gator][conngater] to block and allow peers specified in the `P2PConfig`.

It also sets up a gossiper using the gossip topic `<chainID>+<txTopicSuffix>` (`txTopicSuffix` is defined in [p2p/client.go][client.go]), a Distributed Hash Table (DHT) using the `Seeds` defined in the `P2PConfig` and peer discovery using go-libp2p's `discovery.RoutingDiscovery`.

A P2P client provides an interface `SetTxValidator(p2p.GossipValidator)` for specifying a gossip validator which can define how to handle the incoming `GossipMessage` in the P2P network. The `GossipMessage` represents message gossiped via P2P network (e.g. transaction, Block etc).

```go
// GossipValidator is a callback function type.
type GossipValidator func(*GossipMessage) bool
```

The full nodes define a transaction validator (shown below) as gossip validator for processing the gossiped transactions to add to the mempool, whereas light nodes simply pass a dummy validator as light nodes do not process gossiped transactions.

```go
// newTxValidator creates a pubsub validator that uses the node's mempool to check the
// transaction. If the transaction is valid, then it is added to the mempool
func (n *FullNode) newTxValidator() p2p.GossipValidator {
```

```go
// Dummy validator that always returns a callback function with boolean `false`
func (ln *LightNode) falseValidator() p2p.GossipValidator {
```

## References

[1] [client.go][client.go]

[2] [go-datastore][go-datastore]

[3] [go-libp2p][go-libp2p]

[4] [conngater][conngater]

[client.go]: https://github.com/rollkit/rollkit/blob/main/p2p/client.go#L43
[go-datastore]: https://github.com/ipfs/go-datastore
[go-libp2p]: https://github.com/libp2p/go-libp2p
[conngater]: https://github.com/libp2p/go-libp2p/tree/master/p2p/net/conngater
