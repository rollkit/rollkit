## testapp start

Run the testapp node

```
testapp start [flags]
```

### Options

```
      --chain_id string                                         chain ID (default "rollkit-test")
  -h, --help                                                    help for start
      --rollkit.da.address string                               DA address (host:port) (default "http://localhost:7980")
      --rollkit.da.auth_token string                            DA auth token
      --rollkit.da.block_time duration                          DA chain block time (for syncing) (default 15s)
      --rollkit.da.gas_multiplier float                         DA gas price multiplier for retrying blob transactions
      --rollkit.da.gas_price float                              DA gas price for blob transactions (default -1)
      --rollkit.da.mempool_ttl uint                             number of DA blocks until transaction is dropped from the mempool
      --rollkit.da.namespace string                             DA namespace to submit blob transactions
      --rollkit.da.start_height uint                            starting DA block height (for syncing)
      --rollkit.da.submit_options string                        DA submit options
      --rollkit.db_path string                                  path for the node database (default "data")
      --rollkit.instrumentation.max_open_connections int        maximum number of simultaneous connections for metrics (default 3)
      --rollkit.instrumentation.pprof                           enable pprof HTTP endpoint
      --rollkit.instrumentation.pprof_listen_addr string        pprof HTTP server listening address (default ":6060")
      --rollkit.instrumentation.prometheus                      enable Prometheus metrics
      --rollkit.instrumentation.prometheus_listen_addr string   Prometheus metrics listen address (default ":26660")
      --rollkit.node.aggregator                                 run node in aggregator mode
      --rollkit.node.block_time duration                        block time (for aggregator mode) (default 1s)
      --rollkit.node.lazy_aggregator                            produce blocks only when transactions are available or after lazy block time
      --rollkit.node.lazy_block_time duration                   maximum interval between blocks in lazy aggregation mode (default 1m0s)
      --rollkit.node.light                                      run light client
      --rollkit.node.max_pending_blocks uint                    maximum blocks pending DA confirmation before pausing block production (0 for no limit)
      --rollkit.node.trusted_hash string                        initial trusted hash to start the header exchange service
      --rollkit.p2p.allowed_peers string                        Comma separated list of nodes to whitelist
      --rollkit.p2p.blocked_peers string                        Comma separated list of nodes to ignore
      --rollkit.p2p.listen_address string                       P2P listen address (host:port) (default "/ip4/0.0.0.0/tcp/7676")
      --rollkit.p2p.peers string                                Comma separated list of seed nodes to connect to
      --rollkit.rpc.address string                              RPC server address (host:port) (default "127.0.0.1:7331")
      --rollkit.signer.passphrase string                        passphrase for the signer (required for file signer and if aggregator is enabled)
      --rollkit.signer.path string                              path to the signer file or address (default "config")
      --rollkit.signer.type string                              type of signer to use (file, grpc) (default "file")
```

### Options inherited from parent commands

```
      --home string                 Root directory for application data (default "HOME/.testapp")
      --rollkit.log.format string   Set the log format (text, json) (default "text")
      --rollkit.log.level string    Set the log level (debug, info, warn, error) (default "info")
      --rollkit.log.trace           Enable stack traces in error logs
```

### SEE ALSO

* [testapp](testapp.md)  - The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
