## rollkit completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(rollkit completion bash)

To load completions for every new session, execute once:

#### Linux:

	rollkit completion bash > /etc/bash_completion.d/rollkit

#### macOS:

	rollkit completion bash > $(brew --prefix)/etc/bash_completion.d/rollkit

You will need to start a new shell for this setup to take effect.


```
rollkit completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --home string        directory for config and data (default "HOME/.rollkit")
      --log_level string   set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace              print out full stack trace on errors
```

### SEE ALSO

* [rollkit completion](rollkit_completion.md)	 - Generate the autocompletion script for the specified shell
