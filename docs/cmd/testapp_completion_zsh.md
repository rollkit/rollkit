## testapp completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(testapp completion zsh)

To load completions for every new session, execute once:

#### Linux

	testapp completion zsh > "${fpath[1]}/_testapp"

#### macOS

	testapp completion zsh > $(brew --prefix)/share/zsh/site-functions/_testapp

You will need to start a new shell for this setup to take effect.

```
testapp completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --home string                 Root directory for application data (default "HOME/.testapp")
      --rollkit.log.format string   Set the log format (text, json) (default "text")
      --rollkit.log.level string    Set the log level (debug, info, warn, error) (default "info")
      --rollkit.log.trace           Enable stack traces in error logs
```

### SEE ALSO

* [testapp completion](testapp_completion.md)  - Generate the autocompletion script for the specified shell
