## rollkit start

Run the rollkit node

```
rollkit start [flags]
```

### Options

```
      --db_dir string                    database directory (default "data")
  -h, --help                             help for start
      --p2p.laddr string                 node listen address. (default "tcp://0.0.0.0:26656")
      --proxy_app string                 proxy app address, or one of: 'kvstore', 'persistent_kvstore', 'counter', 'e2e' or 'noop' for local testing. (default "noop")
      --rollkit.aggregator               run node in aggregator mode (default true)
      --rollkit.block_time duration      block time (for aggregator mode) (default 1s)
      --rollkit.da_address string        DA address (host:port) (default ":26650")
      --rollkit.da_block_time duration   DA chain block time (for syncing) (default 15s)
      --rollkit.da_gas_price float       DA gas price for blob transactions (default -1)
      --rollkit.da_namespace string      namespace identifies (8 bytes in hex) (default "0000000000000000")
      --rollkit.da_start_height uint     starting DA block height (for syncing) (default 1)
      --rollkit.lazy_aggregator          wait for transactions, don't build empty blocks
      --rollkit.light                    run light client
      --rollkit.trusted_hash string      initial trusted hash to start the header exchange service
      --rpc.laddr string                 RPC listen address. Port required (default "tcp://127.0.0.1:26657")
      --transport string                 specify abci transport (socket | grpc) (default "socket")
```

### Options inherited from parent commands

```
      --home string        directory for config and data (default "HOME/.rollkit")
      --log_level string   set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace              print out full stack trace on errors
```

### SEE ALSO

* [rollkit](rollkit.md)	 - A modular framework for rollups, with an ABCI-compatible client interface.
