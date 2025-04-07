## testapp completion powershell

Generate the autocompletion script for powershell

### Synopsis

Generate the autocompletion script for powershell.

To load completions in your current shell session:

	testapp completion powershell | Out-String | Invoke-Expression

To load completions for every new session, add the output of the above command
to your powershell profile.


```
testapp completion powershell [flags]
```

### Options

```
  -h, --help              help for powershell
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

* [testapp completion](testapp_completion.md)	 - Generate the autocompletion script for the specified shell
