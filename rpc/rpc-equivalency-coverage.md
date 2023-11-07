# Rollkit RPC Equivalency

## Abstract

Rollkit RPC is a remote procedure call (RPC) service that provides a set of endpoints for interacting with a Rollkit node. It supports various protocols such as URI over HTTP, JSONRPC over HTTP, and JSONRPC over WebSockets.

## Protocol/Component Description

Rollkit RPC serves a variety of endpoints that allow clients to query the state of the blockchain, broadcast transactions, and subscribe to events. The RPC service follows the specifications outlined in the CometBFT [specification].

## Rollkit RPC Functionality Coverage

 Routes                                  | Full Node | Test Coverage |
 --------------------------------------- | --------- | ------------- |
 [Health][health]                        | ✅        | ❌           |
 [Status][status]                        | ✅        | 🚧           |
 [NetInfo][netinfo]                      | ✅        | ❌           |
 [Blockchain][blockchain]                | ✅        | 🚧           |
 [Block][block]                          | ✅        | 🚧           |
 [BlockByHash][blockbyhash]              | ✅        | 🚧           |
 [BlockResults][blockresults]            | ✅        | 🚧           |
 [Commit][commit]                        | ✅        | 🚧           |
 [Validators][validators]                | ✅        | 🚧           |
 [Genesis][genesis]                      | ✅        | 🚧           |
 [GenesisChunked][genesischunked]        | ✅        | 🚧           |
 [ConsensusParams][consensusparams]      | ✅        | 🚧           |
 [UnconfirmedTxs][unconfirmedtxs]        | ✅        | 🚧           |
 [NumUnconfirmedTxs][numunconfirmedtxs]  | ✅        | 🚧           |
 [Tx][tx]                                | ✅        | 🚧           |
 [BroadCastTxSync][broadcasttxsync]      | ✅        | 🚧           |
 [BroadCastTxAsync][broadcasttxasync]    | ✅        | 🚧           |

## Message Structure/Communication Format

The communication format depends on the protocol used. For HTTP-based protocols, the request and response are typically structured as JSON objects. For web socket-based protocols, the messages are sent as JSONRPC requests and responses.

## Assumptions and Considerations

The RPC service assumes that the Rollkit node it interacts with is running and correctly configured. It also assumes that the client is authorized to perform the requested operations.

## Implementation

The implementation of the Rollkit RPC service can be found in the [`rpc/json/service.go`] file in the Rollkit repository.

## References

[1] [CometBFT RPC Specification][specification]
[2] [RPC Service Implementation][`rpc/json/service.go`]

[specification]: https://docs.cometbft.com/v0.38/spec/rpc/
[`rpc/json/service.go`]: https://github.com/rollkit/rollkit/blob/main/rpc/json/service.go
[health]: https://docs.cometbft.com/v0.38/spec/rpc/#health
[status]: https://docs.cometbft.com/v0.38/spec/rpc/#status
[netinfo]: https://docs.cometbft.com/v0.38/spec/rpc/#netinfo
[blockchain]: https://docs.cometbft.com/v0.38/spec/rpc/#blockchain
[block]: https://docs.cometbft.com/v0.38/spec/rpc/#block
[blockbyhash]: https://docs.cometbft.com/v0.38/spec/rpc/#blockbyhash
[blockresults]: https://docs.cometbft.com/v0.38/spec/rpc/#blockresults
[commit]: https://docs.cometbft.com/v0.38/spec/rpc/#commit
[validators]: https://docs.cometbft.com/v0.38/spec/rpc/#validators
[genesis]: https://docs.cometbft.com/v0.38/spec/rpc/#genesis
[genesischunked]: https://docs.cometbft.com/v0.38/spec/rpc/#genesischunked
[consensusparams]: https://docs.cometbft.com/v0.38/spec/rpc/#consensusparams
[unconfirmedtxs]: https://docs.cometbft.com/v0.38/spec/rpc/#unconfirmedtxs
[numunconfirmedtxs]: https://docs.cometbft.com/v0.38/spec/rpc/#numunconfirmedtxs
[tx]: https://docs.cometbft.com/v0.38/spec/rpc/#tx
[broadcasttxsync]: https://docs.cometbft.com/v0.38/spec/rpc/#broadcasttxsync
[broadcasttxasync]: https://docs.cometbft.com/v0.38/spec/rpc/#broadcasttxasync
