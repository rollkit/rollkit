## testapp completion fish

Generate the autocompletion script for fish

### Synopsis

Generate the autocompletion script for the fish shell.

To load completions in your current shell session:

	testapp completion fish | source

To load completions for every new session, execute once:

	testapp completion fish > ~/.config/fish/completions/testapp.fish

You will need to start a new shell for this setup to take effect.

```
testapp completion fish [flags]
```

### Options

```
  -h, --help              help for fish
      --no-descriptions   disable completion descriptions
```

### Options inherited from parent commands

```
      --home string         Root directory for application data (default "/Users/jgimeno/.testapp")
      --log.format string   Set the log format (text, json) (default "plain")
      --log.level string    Set the log level (debug, info, warn, error) (default "info")
      --log.trace           Enable stack traces in error logs
```

### SEE ALSO

* [testapp completion](testapp_completion.md)  - Generate the autocompletion script for the specified shell
