## rollkit start

Run the rollkit node

```
rollkit start [flags]
```

### Options

```
      --ci                                              run node for ci testing
  -h, --help                                            help for start
      --instrumentation.max_open_connections int        maximum number of simultaneous connections for metrics (default 3)
      --instrumentation.prometheus                      enable Prometheus metrics
      --instrumentation.prometheus_listen_addr string   Prometheus metrics listen address (default ":26660")
      --kv-executor-http string                         address for the KV executor HTTP server (empty to disable) (default ":40042")
      --p2p.allowed_peers string                        Comma separated list of nodes to whitelist
      --p2p.blocked_peers string                        Comma separated list of nodes to ignore
      --p2p.listen_address string                       P2P listen address (host:port) (default "/ip4/0.0.0.0/tcp/7676")
      --p2p.seeds string                                Comma separated list of seed nodes to connect to
      --rollkit.aggregator                              run node in aggregator mode
      --rollkit.block_time duration                     block time (for aggregator mode) (default 1s)
      --rollkit.da_address string                       DA address (host:port) (default "http://localhost:26658")
      --rollkit.da_auth_token string                    DA auth token
      --rollkit.da_block_time duration                  DA chain block time (for syncing) (default 15s)
      --rollkit.da_gas_multiplier float                 DA gas price multiplier for retrying blob transactions
      --rollkit.da_gas_price float                      DA gas price for blob transactions (default -1)
      --rollkit.da_mempool_ttl uint                     number of DA blocks until transaction is dropped from the mempool
      --rollkit.da_namespace string                     DA namespace to submit blob transactions
      --rollkit.da_start_height uint                    starting DA block height (for syncing)
      --rollkit.da_submit_options string                DA submit options
      --rollkit.db_path string                          database path relative to root directory (default "data")
      --rollkit.executor_address string                 executor middleware address (host:port) (default "localhost:40041")
      --rollkit.lazy_aggregator                         wait for transactions, don't build empty blocks
      --rollkit.lazy_block_time duration                block time (for lazy mode) (default 1m0s)
      --rollkit.light                                   run light client
      --rollkit.max_pending_blocks uint                 limit of blocks pending DA submission (0 for no limit)
      --rollkit.sequencer_address string                sequencer middleware address (host:port) (default "localhost:50051")
      --rollkit.sequencer_rollup_id string              sequencer middleware rollup ID (default: mock-rollup) (default "mock-rollup")
      --rollkit.trusted_hash string                     initial trusted hash to start the header exchange service
```

### Options inherited from parent commands

```
      --home string        directory for config and data (default "HOME/.rollkit")
      --log_level string   set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace              print out full stack trace on errors
```

### SEE ALSO

* [rollkit](rollkit.md)	 - The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
