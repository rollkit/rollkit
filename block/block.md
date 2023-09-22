# Block

## Abstract

The block manager is a key component of full nodes (including sequencer) that is responsible for block production (in case of sequencer full node) or block syncing (syncing in this context includes retrieving the published blocks from the network, validating them to raise fraud proofs upon validation failure, and updating the state and storing the validated blocks). A full node invokes multiple of the block manager functionalities in parallel, such as:

* block production (in case of sequencer full nodes)
* block publication to DA network
* block retrieval from DA network
* block retrieval from Blockstore (which retrieves blocks from the p2p network)
* block syncing

## Protocol/Component Description

The block manager is initialized using several parameters as defined below:

* `signing key`: used for signing the blocks after production
* `config`: block manager configurations
* `genesis`: initialize the block manager with genesis state
* `localstore`: store blocks and states
* `(mempool, proxyapp, eventbus)`: for initializing the executor (state transition function). mempool is also used in the manager to check for availability of transactions for lazy block production
* `dalc`: to submit and retrieve blocks to DA network  
* `blockstore`: to retrieve blocks from block store (p2p blocksync)

Block manager configuration options:

* `BlockTime`: time interval used for block production and block retrieval from Blockstore (default: 1 second)
* `DABlockTime`: time interval used for both block publication to DA network and block retrieval from DA network (default: 15 seconds)
* `DAStartHeight`: block retrieval from DA network starts from this height
* `NamespaceID`: 8 `byte` unique identifier of the rollup

### Block Production

The sequencer (aka aggregator) type of full nodes run the block production logic. The sequencer can produce blocks under two modes: 1) normal and 2) lazy, which can be specified in the node configurations. In the normal mode, the sequencer runs a timer, which is set to the `BlockTime` configuration parameter of the block manager, and continuously publishes blocks at `BlockTime` intervals. Whereas, in the lazy mode, the manager starts building a block when any transaction becomes available in the mempool. The manager will wait for 1 second timer to finish as part of the block building process, after the first notification of the transaction availability, to collect as many transactions from the mempool as possible. The block manager also notifies the full node after every lazy block building.

#### Building the Block

The block manager of the sequencer nodes perform following steps to produce a block:
* Call `CreateBlock` using executor
* Sign the block to generate commitment
* Call `ApplyBlock` using executor to generate an updated state
* Save the block, validators, and updated state to local store
* Add the newly generated block to `pendingBlocks` queue
* Publish the newly generated block to channels to notify other components of the sequencer node (such as block and header gossip)

### Block Publication to DA Network

The block manager of the sequencer full nodes regularly publish the produced blocks (that are pending in the `pendingBlocks` queue) to DA network using the `DABlockTime` configuration parameter defined in the block manager config. In the event of failure to publish the block to DA network, the manager will perform `maxSubmitAttempts` (default is 30) attempts and a exponential backoff interval between the attempts. The exponential backoff interval starts off at 100 milliseconds and it doubles in the next attempt and capped at `DABlockTime`. A successfull publish event leads to emptying of `pendingBlocks` queue and a failure event leads to proper error reporting.

### Block Retrieval from DA Network

The block manager of the full nodes regularly pulls blocks from the DA network at `DABlockTime` intervals and starts off with a DA height read from the last state stored in the local store or `DAStartHeight` configuration parameter, whichever the latest. The block manager also actively maintains and increments the `daHeight` counter after every DA pull. The pull happens by making the `RetrieveBlocks(daHeight)` request using the Data Availability Light Client (DALC) retriever, which can return either `Success`, `NotFound`, or `Error`. In the event of error, a retry logic kicks in after a delay of 100 milliseconds delay between every retries and 10 retries, after which the error is logged and `daHeight` counter is not incremented, which basically results in the intentional stall of the block retrieval logic. In block `NotFound` scenario does not result in an error and it is acceptable to have no rollup block at every DA height. The retrieval successfully increments the `daHeight` counter in this case. Finally, for the `Success` scenario, first the hashes of the blocks which are successfully retrieved are marked as hard confirmed and are sent for application (or state update). The successful state update triggers a fresh DA and blockstore pulls without respecting the `DABlockTime` and `BlockTime` intervals.

#### About Soft/Hard Confirmations

The block manager retrieves blocks from both p2p network and the underlying DA network, because the blocks are available in the p2p network faster and DA retreival is slower (e.g., 1 second vs 15 seconds). The blocks retrieved from the p2p network are only marked as soft confirmed until the DA retrieval succeeds on those blocks at which time they are marked hard confirmed. The hard confirmations can be considered as finality.

### Block Retrieval from BlockStore (P2P BlockSync)

The block manager of the full nodes regularly pulls blocks from the blockstore (which in turn uses the p2p network for syncing the blocks) at `BlockTime` intervals and starts off with a blockstore height zero. Every time the blockstore height is higher than the last seen height, the newest blocks are pulled from the blockstore and sent for application (or state update), along with updating the last seen blockstore height.

### Block Syncing

The block manager stores and applies the block everytime a new block is retrieved either via blockstore or DA network. Block syncing involves:

* `ApplyBlock` using executor: validate the block, executes the block (applies the transactions), captures the validator updates, and creates an update state.
* `Commit` using executor: commit the execution and changes, update mempool, and publish events
* Store the block, the validators, and the updated state.

## Message Structure/Communication Format

The communication between the block manager and executor:

* `InitChain`: using the genesis, a set of parameters, and validator set to invoke `InitChainSync` on the proxyApp to obtain initial `appHash` and initialize the state.
* `Commit`: commit the execution and changes, update mempool, and publish events.
* `CreateBlock`: prepare a block by polling transactions from mempool.
* `ApplyBlock`: validate the block, execute the block (apply transactions), validator updates, create and return updated state

The communication between the full node and block manager:

* Notify when the block is published
* Notify when the block is done lazy building

## Assumptions and Considerations

* The block manager loads the initial state from the local store and uses genesis if not found in the local store, when the node (re)starts

## Implementation

See [block/manager.go](https://github.com/rollkit/rollkit/blob/main/block/manager.go)

## References
