# KV Executor Benchmark Client

This is a command-line client primarily used for benchmarking the KV executor HTTP server by sending transactions. It can also list the current state of the store.

## Building

```bash
go build -o txclient
```

## Usage

The client runs a transaction benchmark by default.

### Running the Benchmark

```bash
./txclient [flags]
```

By default, the benchmark runs for 30 seconds, sending 10 random key-value transactions every second to `http://localhost:40042`.

**Benchmark Flags:**

* `-duration <duration>`: Total duration for the benchmark (e.g., `1m`, `30s`). Default: `30s`.
* `-interval <duration>`: Interval between sending batches of transactions (e.g., `1s`, `500ms`). Default: `1s`.
* `-tx-per-interval <int>`: Number of transactions to send in each interval. Default: `10`.
* `-addr <url>`: Specify a different server address. Default: `http://localhost:40042`.

**Transaction Data for Benchmark:**

* **Random Data (Default):** If no transaction data flags are provided, the client sends random `key=value` transactions, where keys are 8 characters and values are 16 characters long.
* **Fixed Key/Value:** Use `-key mykey -value myvalue` to send the *same* transaction `mykey=myvalue` repeatedly during the benchmark.
* **Fixed Raw Data:** Use `-raw "myrawdata"` to send the *same* raw transaction data repeatedly during the benchmark.

### List all key-value pairs in the store

```bash
./txclient -list [-addr <url>]
```

This will fetch and display all key-value pairs currently in the KV executor's store. It does not run the benchmark.

## Examples

Run a 1-minute benchmark sending 20 random transactions every 500ms:

```bash
./txclient -duration 1m -interval 500ms -tx-per-interval 20
```

Run a 30-second benchmark repeatedly sending the transaction `user1=alice`:

```bash
./txclient -duration 30s -key user1 -value alice
```

List all values from a specific server:

```bash
./txclient -list -addr http://192.168.1.100:40042
```
