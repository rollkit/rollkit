# Local DA

Local DA implements the [da][da] interface over a Local Data Availability service.

It is intended to be used for testing DA layers without having to set up the actual services.

## Usage

### local-da binary

To build and run the local-da binary:

```sh
make build
./build/local-da
```

This will start the Local DA service with default settings, listening on `localhost:7980` and with a default maximum blob size.

The output should look similar to this (the timestamp and maxBlobSize might vary):

```sh
11:07AM INF NewLocalDA: initialized LocalDA module=local-da
11:07AM INF Listening on host=localhost maxBlobSize=1974272 module=da port=7980
11:07AM INF server started listening on=localhost:7980 module=da
```

Which exposes the [da] interface over JSONRPC and can be accessed with an HTTP client like [xh][xh]:

#### Flags

You can customize the behavior of the `local-da` binary using the following flags:

* `-port <port>`: Specifies the listening port. Default: `7980`.
* `-host <host>`: Specifies the listening address. Default: `localhost`.
* `-listen-all`: If set, the service listens on all network interfaces (`0.0.0.0`) instead of just `localhost`. This allows access from other machines.
* `-max-blob-size <bytes>`: Sets the maximum blob size in bytes that the DA service will accept. Default: `1974272` (which is `64 * 64 * 482`).

**Example with flags:**

To run `local-da` on port `8000`, accessible from any IP, with a max blob size of `1000000` bytes:

```sh
./build/local-da -port 8000 -listen-all -max-blob-size 1000000
```

Output:

```sh
11:07AM INF NewLocalDA: initialized LocalDA module=local-da
11:07AM INF Listening on host=0.0.0.0 maxBlobSize=1974272 module=da port=8000
11:07AM INF server started listening on=0.0.0.0:8000 module=da
```

### MaxBlobSize

```sh
xh http://127.0.0.1:7980 id=1 method=da.MaxBlobSize | jq
```

output:

```sh
{
  "jsonrpc": "2.0",
  "result": 1974272,
  "id": "1"
}
```

#### Submit

```sh
xh http://127.0.0.1:7980 id=1 method=da.Submit 'params[][]=SGVsbG8gd28ybGQh' 'params[]:=-2'  'params[]=AAAAAAAAAAAAAAAAAAAAAAAAAAECAwQFBgcICRA=' | jq
```

output:

```sh
{
    "jsonrpc": "2.0",
    "result": [
        "AQAAAAAAAADfgz5/IP20UeF81iRRzDBu/qC8eXr9DUyplrfXod3VOA=="
    ],
    "id": "1"
}
```

#### Get

```sh
xh http://127.0.0.1:7980 id=1 method=da.Get 'params[][]=AQAAAAAAAADfgz5/IP20UeF81iRRzDBu/qC8eXr9DUyplrfXod3VOA==' 'params[]=AAAAAAAAAAAAAAAAAAAAAAAAAAECAwQFBgcICRA='
```

output:

```sh
{
    "jsonrpc": "2.0",
    "result": [
        "SGVsbG8gd28ybGQh"
    ],
    "id": "1"
}
```

## References

[1] [da][ da]

[2] [xh][xh]

[da]: https://github.com/evstack/ev-node/blob/main/core/da/da.go#L11
[xh]: https://github.com/ducaale/xh
