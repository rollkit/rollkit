## rollkit completion zsh

Generate the autocompletion script for zsh

### Synopsis

Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment you will need
to enable it.  You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(rollkit completion zsh)

To load completions for every new session, execute once:

#### Linux

	rollkit completion zsh > "${fpath[1]}/_rollkit"

#### macOS

	rollkit completion zsh > $(brew --prefix)/share/zsh/site-functions/_rollkit

You will need to start a new shell for this setup to take effect.

```
rollkit completion zsh [flags]
```

### Options

```
  -h, --help              help for zsh
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --home string        directory for config and data (default "HOME/.rollkit")
      --log_level string   set the log level; default is info. other options include debug, info, error, none (default "info")
      --trace              print out full stack trace on errors
```

### SEE ALSO

* [rollkit completion](rollkit_completion.md)  - Generate the autocompletion script for the specified shell
