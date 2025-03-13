## rollkit

The first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.

### Synopsis


Rollkit is the first sovereign rollup framework that allows you to launch a sovereign, customizable blockchain as easily as a smart contract.
The rollkit-cli uses the environment variable "RKHOME" to point to a file path where the node keys, config, and data will be stored. 
If a path is not specified for RKHOME, the rollkit command will create a folder "~/.rollkit" where it will store said data.


### Options

```
  -h, --help                help for rollkit
      --log_format string   set the log format; options include plain and json (default "plain")
      --log_level string    set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace               print out full stack trace on errors
```

### SEE ALSO

* [rollkit completion](rollkit_completion.md)	 - Generate the autocompletion script for the specified shell
* [rollkit docs-gen](rollkit_docs-gen.md)	 - Generate documentation for rollkit CLI
* [rollkit rebuild](rollkit_rebuild.md)	 - Rebuild rollup entrypoint
* [rollkit start](rollkit_start.md)	 - Run the rollkit node
* [rollkit toml](rollkit_toml.md)	 - TOML file operations
* [rollkit version](rollkit_version.md)	 - Show version info
