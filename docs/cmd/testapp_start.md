## testapp start

Run the rollkit node

```
testapp start [flags]
```

### Options

```
      --ci                                              run node for ci testing
      --config_dir string                               chain configuration directory (default "config")
      --da.address string                               DA address (host:port) (default "http://localhost:26658")
      --da.auth_token string                            DA auth token
      --da.block_time duration                          DA chain block time (for syncing) (default 15s)
      --da.gas_multiplier float                         DA gas price multiplier for retrying blob transactions
      --da.gas_price float                              DA gas price for blob transactions (default -1)
      --da.mempool_ttl uint                             number of DA blocks until transaction is dropped from the mempool
      --da.namespace string                             DA namespace to submit blob transactions
      --da.start_height uint                            starting DA block height (for syncing)
      --da.submit_options string                        DA submit options
      --db_path string                                  database path relative to root directory (default "data")
  -h, --help                                            help for start
      --instrumentation.max_open_connections int        maximum number of simultaneous connections for metrics (default 3)
      --instrumentation.pprof                           enable pprof HTTP endpoint
      --instrumentation.pprof_listen_addr string        pprof HTTP server listening address (default ":6060")
      --instrumentation.prometheus                      enable Prometheus metrics
      --instrumentation.prometheus_listen_addr string   Prometheus metrics listen address (default ":26660")
      --node.aggregator                                 run node in aggregator mode (default true)
      --node.block_time duration                        block time (for aggregator mode) (default 1s)
      --node.executor_address string                    executor middleware address (host:port) (default "localhost:40041")
      --node.lazy_aggregator                            produce blocks only when transactions are available or after lazy block time
      --node.lazy_block_time duration                   maximum interval between blocks in lazy aggregation mode (default 1m0s)
      --node.light                                      run light client
      --node.max_pending_blocks uint                    maximum blocks pending DA confirmation before pausing block production (0 for no limit)
      --node.sequencer_address string                   sequencer middleware address (host:port) (default "localhost:50051")
      --node.sequencer_rollup_id string                 sequencer middleware rollup ID (default: mock-rollup) (default "mock-rollup")
      --node.trusted_hash string                        initial trusted hash to start the header exchange service
      --p2p.allowed_peers string                        Comma separated list of nodes to whitelist
      --p2p.blocked_peers string                        Comma separated list of nodes to ignore
      --p2p.listen_address string                       P2P listen address (host:port) (default "/ip4/0.0.0.0/tcp/7676")
      --p2p.seeds string                                Comma separated list of seed nodes to connect to
      --rpc.address string                              RPC server address (host) (default "127.0.0.1")
      --rpc.port uint16                                 RPC server port (default 7331)
```

### Options inherited from parent commands

```
      --home string         Root directory for application data (default "HOME/.testapp")
      --log.format string   Set the log format (text, json) (default "plain")
      --log.level string    Set the log level (debug, info, warn, error) (default "info")
      --log.trace           Enable stack traces in error logs
```

### SEE ALSO

* [testapp](testapp.md)	 - The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
