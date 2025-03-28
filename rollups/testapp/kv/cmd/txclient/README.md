# KV Executor Transaction Client

This is a command-line client for interacting with the KV executor HTTP server.

## Building

```bash
go build -o txclient
```

## Usage

The client supports the following operations:

### Submit a transaction (key-value format)

```bash
./txclient -key mykey -value myvalue
```

This will create and submit a transaction in the format `mykey=myvalue` to the KV executor.

### Submit a raw transaction

```bash
./txclient -raw "mykey=myvalue"
```

This allows you to directly specify the transaction data. For the KV executor, it should be in the format `key=value`.

### List all key-value pairs in the store

```bash
./txclient -list
```

This will fetch and display all key-value pairs currently in the KV executor's store.

### Specify a different server address

By default, the client connects to `http://localhost:40042`. You can specify a different address:

```bash
./txclient -addr http://localhost:40042 -key mykey -value myvalue
```

## Examples

Set a value:

```bash
./txclient -key user1 -value alice
```

Set another value:

```bash
./txclient -key user2 -value bob
```

List all values:

```bash
./txclient -list
```

Output:

```json
{
  "user1": "alice",
  "user2": "bob"
}
```
