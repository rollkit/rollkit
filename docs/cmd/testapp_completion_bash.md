## testapp completion bash

Generate the autocompletion script for bash

### Synopsis

Generate the autocompletion script for the bash shell.

This script depends on the 'bash-completion' package.
If it is not installed already, you can install it via your OS's package manager.

To load completions in your current shell session:

	source <(testapp completion bash)

To load completions for every new session, execute once:

#### Linux:

	testapp completion bash > /etc/bash_completion.d/testapp

#### macOS:

	testapp completion bash > $(brew --prefix)/etc/bash_completion.d/testapp

You will need to start a new shell for this setup to take effect.


```
testapp completion bash
```

### Options

```
  -h, --help              help for bash
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --home string         Root directory for application data (default "HOME/.testapp")
      --log.format string   Set the log format (text, json) (default "plain")
      --log.level string    Set the log level (debug, info, warn, error) (default "info")
      --log.trace           Enable stack traces in error logs
```

### SEE ALSO

* [testapp completion](testapp_completion.md)	 - Generate the autocompletion script for the specified shell
