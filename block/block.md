# Block

## Abstract

The block manager is a key component of full nodes and is responsible for block production or block syncing depending on the node type. Block syncing in this context includes retrieving the published blocks from the network (p2p network or DA network), validating them to raise fraud proofs upon validation failure, updating the state, and storing the validated blocks. A full node invokes multiple block manager functionalities in parallel, such as:

* block production (only for sequencer full nodes)
* block publication to DA network
* block retrieval from DA network
* block retrieval from Blockstore (which retrieves blocks from the p2p network)
* block syncing

## Protocol/Component Description

The block manager is initialized using several parameters as defined below:

**Name**|**Type**|**Description**
|-----|-----|-----|
signing key|crypto.PrivKey|used for signing a block after it is created
config|config.BlockManagerConfig|block manager configurations (see config options below)
genesis|*cmtypes.GenesisDoc|initialize the block manager with genesis state (genesis configuration defined in `config/genesis.json` file under the app directory)
store|store.Store|local datastore for storing rollup blocks and states (default local store path is `$db_dir/rollkit` and `db_dir` specified in the `config.toml` file under the app directory)
mempool, proxyapp, eventbus|mempool.Mempool, proxy.AppConnConsensus, *cmtypes.EventBus|for initializing the executor (state transition function). mempool is also used in the manager to check for availability of transactions for lazy block production
dalc|da.DataAvailabilityLayerClient|the data availability light client used to submit and retrieve blocks to DA network
blockstore|*goheaderstore.Store[*types.Block]|to retrieve blocks gossiped over the p2p network

Block manager configuration options:

|Name|Type|Description|
|-----|-----|-----|
|BlockTime|time.Duration|time interval used for block production and block retrieval from block store (default: 1 second)|
|DABlockTime|time.Duration|time interval used for both block publication to DA network and block retrieval from DA network (default: 15 seconds)|
|DAStartHeight|uint64|block retrieval from DA network starts from this height|
|NamespaceID|bytes|8 `byte` unique identifier of the rollup|

### Block Production

When the full node is operating as a sequencer (aka aggregator), the block manager runs the block production logic. There are two modes of block production, which can be specified in the block manager configurations: `normal` and `lazy`.

In `normal` mode, the block manager runs a timer, which is set to the `BlockTime` configuration parameter, and continuously produces blocks at `BlockTime` intervals.

In `lazy` mode, the block manager starts building a block when any transaction becomes available in the mempool. After the first notification of the transaction availability, the manager will wait for a 1 second timer to finish, in order to collect as many transactions from the mempool as possible. The 1 second delay is chosen in accordance with the default block time of 1s. The block manager also notifies the full node after every lazy block building.

#### Building the Block

The block manager of the sequencer nodes performs the following steps to produce a block:

* Call `CreateBlock` using executor
* Sign the block using `signing key` to generate commitment
* Call `ApplyBlock` using executor to generate an updated state
* Save the block, validators, and updated state to local store
* Add the newly generated block to `pendingBlocks` queue
* Publish the newly generated block to channels to notify other components of the sequencer node (such as block and header gossip)

### Block Publication to DA Network

The block manager of the sequencer full nodes regularly publishes the produced blocks (that are pending in the `pendingBlocks` queue) to the DA network using the `DABlockTime` configuration parameter defined in the block manager config. In the event of failure to publish the block to the DA network, the manager will perform `maxSubmitAttempts` (default is 30) attempts and an exponential backoff interval between the attempts. The exponential backoff interval starts off at 100 milliseconds and it doubles in the next attempt and capped at `DABlockTime`. A successful publish event leads to the emptying of `pendingBlocks` queue and a failure event leads to proper error reporting and without emptying of `pendingBlocks` queue.

### Block Retrieval from DA Network

The block manager of the full nodes regularly pulls blocks from the DA network at `DABlockTime` intervals and starts off with a DA height read from the last state stored in the local store or `DAStartHeight` configuration parameter, whichever is the latest. The block manager also actively maintains and increments the `daHeight` counter after every DA pull. The pull happens by making the `RetrieveBlocks(daHeight)` request using the Data Availability Light Client (DALC) retriever, which can return either `Success`, `NotFound`, or `Error`. In the event of an error, a retry logic kicks in after a delay of 100 milliseconds delay between every retry and after 10 retries, an error is logged and the `daHeight` counter is not incremented, which basically results in the intentional stalling of the block retrieval logic. In the block `NotFound` scenario, there is no error as it is acceptable to have no rollup block at every DA height. The retrieval successfully increments the `daHeight` counter in this case. Finally, for the `Success` scenario, first, blocks that are successfully retrieved are marked as hard confirmed and are sent to be applied (or state update). A successful state update triggers fresh DA and block store pulls without respecting the `DABlockTime` and `BlockTime` intervals.

#### About Soft/Hard Confirmations

The block manager retrieves blocks from both the p2p network and the underlying DA network because the blocks are available in the p2p network faster and DA retrieval is slower (e.g., 1 second vs 15 seconds). The blocks retrieved from the p2p network are only marked as soft confirmed until the DA retrieval succeeds on those blocks and they are marked hard confirmed. The hard confirmations can be considered to have a higher level of finality.

### Block Retrieval from BlockStore (P2P BlockSync)

The block manager of the full nodes regularly pulls blocks from the block store (which in turn uses the p2p network for syncing the blocks) at `BlockTime` intervals and starts off with a block store height of zero. Every time the block store height is higher than the last seen height, the newest blocks are pulled from the block store and sent to be applied (or state update), along with updating the last seen block store height.

### Block Syncing

The block manager stores and applies the block every time a new block is retrieved either via the blockstore or DA network. Block syncing involves:

* `ApplyBlock` using executor: validates the block, executes the block (applies the transactions), captures the validator updates, and creates an updated state.
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

* The block manager loads the initial state from the local store and uses genesis if not found in the local store, when the node (re)starts.
* The default mode for sequencer nodes is normal (not lazy).
* The sequencer can produce empty blocks.
* The block manager uses persistent storage (disk) when the `root_dir` and `db_path` configuration parameters are specified in `config.toml` file under the app directory. If these configuration parameters are not specified, the in-memory storage is used, which will not be persistent if the node stops.
* The block manager does not re-apply the block again (in other words, create a new updated state and persist it) when a block was initially applied using p2p block sync, but later was hard confirmed by DA retrieval. The block is only set hard confirmed in this case.

## Implementation

See [block/manager.go](https://github.com/rollkit/rollkit/blob/main/block/manager.go)

## References
