# Using Optimint `Node` as replacement of Tendermint `Node`

Replacing on the `Node` level gives much flexibility. Still, significant amount of code can be reused, and there is no need to refactor lazyledger-core.
Cosmos SDK is tigtly coupled with Tendermint with regards to node creation, RPC, app initialization, etc. De-coupling requires big refactoring of cosmos-sdk.

There are known issues related to Tendermint RPC communication. 

## Replacing Tendermint `Node`
Tendermint `Node` is a struct. It's used directly in cosmos-sdk (not via interface).
We don't need to introduce common interface `Node`s, because the plan is to use scafolding tool in the feature, so we can make any required changes in cosmos-sdk.
### Interface required by cosmos-sdk:
* BaseService (struct):
  * Service (interface)
	  * Start()
	  * IsRunning()
	  * Stop()
  * Logger
* Direct access:
  * ConfigureRPC()
  * EventBus()

## Alternative approaches
### Create RPC from scratch
* Pros:
  * May be possible to avoid Tendermint issues
  * Should be possible to avoid dependency on Tendermint in Optimint
  * Changes probably limited to cosmos-sdk (not required in tendermint/lazyledger-core) 
* Cons:
  * Reinventing the wheel
  * Requires bigger, much more complicated changes in cosmos-sdk
  * Probably can't upstream such changes to cosmos-sdk

## `tendermint` vs `lazyledger-core`
Right now, either `tendermint` or `lazyledger-core` can be used for base types (including interfaces). 
Similarly, vanilla `cosomos-sdk` (not a fork under lazyledger organization) can be used as a base for ORU client.
`lazyledger-core` is a repository created because of needs related to lazyledger client, not optimistic rollups client.
On the other hand, some of the functionality will be shared between both clients. This will have to be resolved later in time.
Using 'vanilla' repositories (not forks) probably will make easier to upstream changes if required, and will make scaffolding
easier.

## Development
For development, there is `master-optimint` branch in `cosmos-sdk` repository. Versions with `-optimint` suffix will be released from this branch for easier dependency management during development.

