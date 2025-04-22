## testapp unsafe-clean

Remove all contents of the data directory (DANGEROUS: cannot be undone)

### Synopsis

Removes all files and subdirectories in the node's data directory.
This operation is unsafe and cannot be undone. Use with caution!

```
testapp unsafe-clean [flags]
```

### Options

```
  -h, --help   help for unsafe-clean
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
