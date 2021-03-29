# Peer Discovery

Libp2p provides multiple ways to discover peers (DHT, mDNS). Currently there are no plans to support mDNS (as it's limited to local networks).

## Proposed network architecture
1. There will be a set of well-known, application-agnostic seed nodes. Every optimint client will be able to connect to such node, addresses will be saved in configuration.
    * This does not limit applications which can create independent networks, with separate set of seed nodes.
2. Nodes in the network will serve DHT. It will be used for active peer discovery. Client of each ORU network will be able to find other peers in this particular network.
    * All nodes will cooperate on the same DHT.
    * ChainID will be used to advertise that client works in given ORU network.
3. Nodes from multiple networks will help with peer discovery (via single DHT).
4. After connecting to nodes found in DHT, GossipSub will handle peer lists for clients.

### Pros
* Shared DHT should make it easier to find peers.
* Use of existing libraries.

### Cons
* There may be some overhead for clients to handle DHT requests from other ORU networks.

## Alternatives
1. IPFS DHT for peer discovery.
2. Custom peer-exchange protocol.
