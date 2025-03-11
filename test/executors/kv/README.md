# KV Executor

This is a simple key-value store executor implementation for testing Rollkit nodes.

## HTTP Server

The KV executor includes an HTTP server that allows submitting transactions and interacting with the key-value store directly. The server is started automatically when using the KV executor with the Rollkit node.

### Configuration

The HTTP server address can be configured using the `--kv-executor-http` flag when starting the Rollkit node. For example:

```bash
./rollkit run --kv-executor-http=:40042
```

To disable the HTTP server, set the flag to an empty string:

```bash
./rollkit run --kv-executor-http=
```

### Server Lifecycle

The HTTP server starts when the Rollkit node starts and is automatically shut down when the node stops (including when receiving CTRL+C or other termination signals). The server is context-aware, meaning:

1. It gracefully handles in-progress requests during shutdown
2. It automatically shuts down when the node's context is cancelled
3. There's a 5-second timeout for shutdown to complete

### API Endpoints

The HTTP server provides the following endpoints:

- `POST /tx`: Submit a transaction to the KV executor.
  - Request body should contain the transaction data.
  - For consistency with the KV executor's transaction format, it's recommended to use transactions in the format `key=value`.
  - Returns HTTP 202 (Accepted) if the transaction is accepted.

- `GET /kv?key=<key>`: Get the value for a specific key.
  - Returns the value as plain text if the key exists.
  - Returns HTTP 404 if the key doesn't exist.

- `POST /kv`: Set a key-value pair directly.
  - Request body should be a JSON object with `key` and `value` fields.
  - Example: `{"key": "mykey", "value": "myvalue"}`

- `GET /store`: Get all key-value pairs in the store.
  - Returns a JSON object with all key-value pairs.

## CLI Client

A simple CLI client is provided to interact with the KV executor HTTP server. To build the client:

```bash
cd test/executors/kv/cmd/txclient
go build -o txclient
```

### Usage

Submit a transaction with key and value:

```bash
./txclient -key mykey -value myvalue
```

Submit a raw transaction:

```bash
./txclient -raw "mykey=myvalue"
```

List all key-value pairs in the store:

```bash
./txclient -list
```

Specify a different server address:

```bash
./txclient -addr http://localhost:40042 -key mykey -value myvalue
```

## Transaction Format

The KV executor expects transactions in the format `key=value`. For example:

- `mykey=myvalue`
- `foo=bar`

When a transaction is executed, the key-value pair is added to the store.
