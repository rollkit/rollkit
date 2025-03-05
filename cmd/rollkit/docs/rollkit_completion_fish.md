## rollkit completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	rollkit completion fish | source

To load completions for every new session, execute once:

	rollkit completion fish > ~/.config/fish/completions/rollkit.fish

You will need to start a new shell for this setup to take effect.

```
rollkit completion fish [flags]
```

### Options

```
  -h, --help              help for fish
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
