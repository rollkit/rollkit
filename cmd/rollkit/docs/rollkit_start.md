## rollkit start

Run the rollkit node

```
rollkit start [flags]
```

### Options

```
      --abci string                                     specify abci transport (socket | grpc) (default "socket")
      --consensus.create_empty_blocks                   set this to false to only produce blocks when there are txs or when the AppHash changes (default true)
      --consensus.create_empty_blocks_interval string   the possible interval between empty blocks (default "0s")
      --consensus.double_sign_check_height int          how many blocks to look back to check existence of the node's consensus votes before joining consensus
      --db_backend string                               database backend: goleveldb | cleveldb | boltdb | rocksdb | badgerdb (default "goleveldb")
      --db_dir string                                   database directory (default "data")
      --genesis_hash bytesHex                           optional SHA-256 hash of the genesis file
  -h, --help                                            help for start
      --moniker string                                  node name (default "Your Computer Username")
      --p2p.external-address string                     ip:port address to advertise to peers for them to dial
      --p2p.laddr string                                node listen address. (0.0.0.0:0 means any interface, any port) (default "tcp://0.0.0.0:26656")
      --p2p.persistent_peers string                     comma-delimited ID@host:port persistent peers
      --p2p.pex                                         enable/disable Peer-Exchange (default true)
      --p2p.private_peer_ids string                     comma-delimited private peer IDs
      --p2p.seed_mode                                   enable/disable seed mode
      --p2p.seeds string                                comma-delimited ID@host:port seed nodes
      --p2p.unconditional_peer_ids string               comma-delimited IDs of unconditional peers
      --priv_validator_laddr string                     socket address to listen on for connections from external priv_validator process
      --proxy_app string                                proxy app address, or one of: 'kvstore', 'persistent_kvstore' or 'noop' for local testing. (default "tcp://127.0.0.1:26658")
      --rollkit.aggregator                              run node in aggregator mode
      --rollkit.block_time duration                     block time (for aggregator mode) (default 1s)
      --rollkit.da_address string                       DA address (host:port) (default ":26650")
      --rollkit.da_auth_token string                    DA auth token
      --rollkit.da_block_time duration                  DA chain block time (for syncing) (default 15s)
      --rollkit.da_gas_multiplier float                 DA gas price multiplier for retrying blob transactions (default -1)
      --rollkit.da_gas_price float                      DA gas price for blob transactions (default -1)
      --rollkit.da_namespace string                     DA namespace to submit blob transactions
      --rollkit.da_start_height uint                    starting DA block height (for syncing)
      --rollkit.lazy_aggregator                         wait for transactions, don't build empty blocks
      --rollkit.light                                   run light client
      --rollkit.trusted_hash string                     initial trusted hash to start the header exchange service
      --rpc.grpc_laddr string                           GRPC listen address (BroadcastTx only). Port required
      --rpc.laddr string                                RPC listen address. Port required (default "tcp://127.0.0.1:26657")
      --rpc.pprof_laddr string                          pprof listen address (https://golang.org/pkg/net/http/pprof)
      --rpc.unsafe                                      enabled unsafe rpc methods
      --transport string                                specify abci transport (socket | grpc) (default "socket")
```

### Options inherited from parent commands

```
      --home string        directory for config and data (default "HOME/.rollkit")
      --log_level string   set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace              print out full stack trace on errors
```

### SEE ALSO

* [rollkit](rollkit.md)	 - A modular framework for rollups, with an ABCI-compatible client interface.
