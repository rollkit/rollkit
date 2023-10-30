# Rollkit

A modular framework for rollups, with an ABCI-compatible client interface. For more in-depth information about Rollkit, please visit our [website](https://rollkit.dev).

[![build-and-test](https://github.com/rollkit/rollkit/actions/workflows/test.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/test.yml)
[![golangci-lint](https://github.com/rollkit/rollkit/actions/workflows/lint.yml/badge.svg)](https://github.com/rollkit/rollkit/actions/workflows/lint.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/rollkit/rollkit)](https://goreportcard.com/report/github.com/rollkit/rollkit)
[![codecov](https://codecov.io/gh/rollkit/rollkit/branch/main/graph/badge.svg?token=CWGA4RLDS9)](https://codecov.io/gh/rollkit/rollkit)
[![GoDoc](https://godoc.org/github.com/rollkit/rollkit?status.svg)](https://godoc.org/github.com/rollkit/rollkit)

## Building From Source

Requires Go version >= 1.19.

To build:

```sh
git clone https://github.com/rollkit/rollkit.git
cd rollkit 
go build -v ./...
```

## Building With Rollkit

While Rollkit is a modular framework that aims to be compatible with a wide
range of data availability layers, settlement layers, and execution
environments, the most supported development environment is building on Celestia
as a data availability layer.

### Building On Celestia

There are currently 2 ways to build on Celestia:

1. Using a local development environment with [local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet)
1. Using the Arabica or Mocha Celestia testnet

#### Compatibility

| network               | rollkit    | celestia-node | celestia-app |
|-----------------------|------------|---------------|--------------|
| local-celestia-devnet | v0.9.0     | v0.11.0-rc6   | v1.0.0-rc7   |
| arabica               | v0.9.0     | v0.11.0-rc6   | v1.0.0-rc7   |

| rollkit/cosmos-sdk                          | rollkit/cometbft                   | rollkit    |
|---------------------------------------------|------------------------------------|------------|
| v0.46.13-rollkit-v0.9.0-no-fraud-proofs     | v0.0.0-20230524013049-75272ebaee38 | v0.9.0     |
| v0.45.16-rollkit-v0.9.0-no-fraud-proofs     | v0.0.0-20230524013001-2968c8b8b121 | v0.9.0     |

#### Local Development Environment

The Rollkit v0.9.0 release is compatible with the
[local-celestia-devnet](https://github.com/rollkit/local-celestia-devnet)
[oolong](https://github.com/rollkit/local-celestia-devnet/releases) release. This version combination is compatible with
[celestia-app](https://github.com/celestiaorg/celestia-app) v1.0.0-rc7 and
[celestia-node](https://github.com/celestiaorg/celestia-node) v0.11.0-rc6.

#### Arabica and Mocha Testnets

The Rollkit v0.9.0 release is compatible with [Arabica](https://docs.celestia.org/nodes/arabica-devnet/) devnet which is running [celestia-app](https://github.com/celestiaorg/celestia-app) v1.0.0-rc7 and
[celestia-node](https://github.com/celestiaorg/celestia-node) v0.11.0-rc6.

> :warning: **Rollkit v0.9.0 is not tested for compatibility with latest releases of Mocha.** :warning:

### Tools

1. Install [golangci-lint](https://golangci-lint.run/usage/install/)
1. Install [markdownlint](https://github.com/DavidAnson/markdownlint)
1. Install [hadolint](https://github.com/hadolint/hadolint)
1. Install [yamllint](https://yamllint.readthedocs.io/en/stable/quickstart.html)

## Helpful Commands

```sh
# Run unit tests
make test

# Generate protobuf files (requires Docker)
make proto-gen

# Run linters (requires golangci-lint, markdownlint, hadolint, and yamllint)
make lint

# Lint protobuf files (requires Docker and buf)
make proto-lint

```

## Contributing

We welcome your contributions! Everyone is welcome to contribute, whether it's in the form of code,
documentation, bug reports, feature requests, or anything else.

If you're looking for issues to work on, try looking at the [good first issue list](https://github.com/rollkit/rollkit/issues?q=is%3Aissue+is%3Aopen+label%3A%22good+first+issue%22). Issues with this tag are suitable for a new external contributor and is a great way to find something you can help with!

See [the contributing guide](./CONTRIBUTING.md) for more details.

Please join our [Community Discord](https://discord.com/invite/YsnTPcSfWQ) to ask questions, discuss your ideas, and connect with other contributors.

## Dependency Graph

To see our progress and a possible future of Rollkit visit our [Dependency Graph](./docs/specification/rollkit-dependency-graph.md).

## Code of Conduct

See our Code of Conduct [here](https://docs.celestia.org/community/coc).

## Building on avail
There are currently 2 ways to build on Avail:

1. Using a local development environment
2. Using the Avail kate testnet

### Avail-da

This package implements DataAvailabilityLayerClient interface in rollkit

### Installation for Local Development Environment

### Required nodes to run

#### 1. Data availability node

* clone the repo 

    ``` https://github.com/availproject/avail.git ```
* go to root folder

    ``` cd avail ```

    ``` git checkout v1.6.2-rc1 ```
* compile 

    ``` cargo build --release -p data-avail ```
* run node

    ``` cargo run --release -p data-avail -- --dev --tmp ```
  
    logs will appear as below :

     ```
    Finished release [optimized] target(s) in 5.05s
    Running `target/release/data-avail --dev --tmp`
    2023-09-12 10:57:30 Avail Node
    2023-09-12 10:57:30 :v:  version 1.6.2-bb4cc104b25
    2023-09-12 10:57:30 :heart:  by Anonymous, 2017-2023
    2023-09-12 10:57:30 :clipboard: Chain specification: Avail Local Solo
    2023-09-12 10:57:30 :label:  Node name: ragged-dolls-4393
    2023-09-12 10:57:30 :bust_in_silhouette: Role: AUTHORITY
    2023-09-12 10:57:30 :floppy_disk: Database: RocksDb at /tmp/substrateCM4lMG/chains/Avail Local/db/full
    2023-09-12 10:57:30 :chains:  Native runtime: data-avail-11 (data-avail-0.tx1.au11)
    2023-09-12 10:57:32 [0] :money_with_wings: generated 1 npos voters, 1 from validators and 0 nominators
    2023-09-12 10:57:32 [0] :money_with_wings: generated 1 npos targets
    2023-09-12 10:57:32 :hammer: Initializing Genesis block/state (state: 0xb7fe…2d6d, header-hash: 0xa6ee…d7be)
    2023-09-12 10:57:32 :older_man: Loading GRANDPA authority set from genesis on what appears to be first startup.
    2023-09-12 10:57:33 :baby: Creating empty BABE epoch changes on what appears to be first startup.
    2023-09-12 10:57:33 :label:  Local node identity is: 12D3KooWEBa9XwkepFqfWv1Y1gQMDa6QAxgVPPpFtvxRHeCAhpvG
    2023-09-12 10:57:33 Prometheus metrics extended with avail metrics
    2023-09-12 10:57:33 :computer: Operating system: linux
    2023-09-12 10:57:33 :computer: CPU architecture: x86_64
    2023-09-12 10:57:33 :computer: Target environment: gnu
    2023-09-12 10:57:33 :computer: CPU: 13th Gen Intel(R) Core(TM) i5-1335U
    2023-09-12 10:57:33 :computer: CPU cores: 10
    2023-09-12 10:57:33 :computer: Memory: 15670MB
    2023-09-12 10:57:33 :computer: Kernel: 6.2.0-32-generic
    2023-09-12 10:57:33 :computer: Linux distribution: Ubuntu 22.04.3 LTS
    2023-09-12 10:57:33 :computer: Virtual machine: no
    2023-09-12 10:57:33 :package: Highest known block at #0
    2023-09-12 10:57:33 Running JSON-RPC HTTP server: addr=127.0.0.1:9933, allowed origins=["*"]
    2023-09-12 10:57:33 :part_alternation_mark: Prometheus exporter started at 127.0.0.1:9615
    2023-09-12 10:57:33 Running JSON-RPC WS server: addr=127.0.0.1:9944, allowed origins=["*"]
    2023-09-12 10:57:33 :checkered_flag: CPU score: 944.65 MiBs
    2023-09-12 10:57:33 :checkered_flag: Memory score: 18.86 GiBs
    2023-09-12 10:57:33 :checkered_flag: Disk score (seq. writes): 1.34 GiBs
    2023-09-12 10:57:33 :checkered_flag: Disk score (rand. writes): 736.10 MiBs
    2023-09-12 10:57:33 :baby: Starting BABE Authorship worker
    2023-09-12 10:57:38 :zzz: Idle (0 peers), best: #0 (0xa6ee…d7be), finalized #0 (0xa6ee…d7be), :arrow_down: 0 :arrow_up: 0
    2023-09-12 10:57:40 :raised_hands: Starting consensus session on top of parent 0xa6ee9fe89d69cf0c3f26922b961d8a8db7fbb9e973b7820f370412189810d7be
    2023-09-12 10:57:40 :gift: Prepared block for proposing at 1 (16 ms) [hash: 0x67901da0d9cac880b6f75086ee86a48417c8a59c90b44c130257367d17c2863e; parent_hash: 0xa6ee…d7be; extrinsics (1): [0xc910…d9fb]]
    2023-09-12 10:57:40 :bookmark: Pre-sealed block for proposal at 1. Hash now 0xd080fd49331fe59b1d400ee4a55e128d993241287601ae8f57b1447c7d16f66c, previously 0x67901da0d9cac880b6f75086ee86a48417c8a59c90b44c130257367d17c2863e
        ```

#### 2. Avail light client

    
* clone the repo

    ``` https://github.com/availproject/avail-light.git ```

* go to root folder

    ``` cd avail-light ```

    ``` git checkout v1.4.4 ```
     
* create one yaml configuration file ```config1.yaml``` in the root of the project & put following content.

    ```
        log_level = "info"
        http_server_host = "127.0.0.1"
        http_server_port = "7000"
        libp2p_seed = 1
        libp2p_port = "37000"
        full_node_ws = ["ws://127.0.0.1:9944"]
        app_id = 1
        confidence = 92.0
        avail_path = "avail_path"
        prometheus_port = 9520
        bootstraps = [] 
            
    ```

* run node with first configuration file 

    ```cargo run -- -c config1.yaml ```
        
  logs will appear as below:

  ```
        warning: variant `PutKadRecord` is never constructed
        --> src/network/client.rs:355:2
            |
        335 | pub enum Command {
            |          ------- variant in this enum
        ...
        355 |     PutKadRecord {
            |     ^^^^^^^^^^^^
            |
            = note: `Command` has a derived impl for the trait `Debug`, but this is intentionally ignored during dead code analysis
            = note: `#[warn(dead_code)]` on by default

        warning: `avail-light` (bin "avail-light") generated 1 warning
            Finished dev [unoptimized + debuginfo] target(s) in 4.09s
            Running `target/debug/avail-light -c config1.yaml`
        2023-09-12T05:44:23.705197Z  INFO avail_light::telemetry: Metrics server on http://0.0.0.0:9520/metrics
        2023-09-12T05:44:23.818184Z  INFO avail_light::http: RPC running on http://127.0.0.1:7000
        2023-09-12T05:44:23.818947Z  INFO Server::run{addr=127.0.0.1:7000}: warp::server: listening on http://127.0.0.1:7000
        2023-09-12T05:44:23.820724Z  INFO avail_light::network: Local peer id: PeerId("12D3KooWMD1Sg5UyNEGxCPQGP9tsKkeCY2tn1dKb5GRQx1LZPi6o"). Public key: Ed25519(PublicKey(compressed): a93d7b734ba3a54efa8b9847c6368c6333d44c4cec93aed9ff8aeffef5ce4).
        2023-09-12T05:44:23.864078Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/127.0.0.1/udp/37000/quic-v1"
        2023-09-12T05:44:23.865120Z  INFO avail_light: No bootstrap nodes, waiting for first peer to connect...
        2023-09-12T05:44:23.865223Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/192.168.1.40/udp/37000/quic-v1"
        2023-09-12T05:44:23.865553Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/172.17.0.1/udp/37000/quic-v1"
        2023-09-12T05:44:23.868076Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/127.0.0.1/tcp/37000"
        2023-09-12T05:44:23.868865Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/192.168.1.40/tcp/37000"
        2023-09-12T05:44:23.869487Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/172.17.0.1/tcp/37000"
  ```

* copy the local peer id in the above logs and Run another LC, with another config (copy the above config) and change the port for server, libp2p, prometheus and the avail_path, change the first argument in the bootstraps to the  address of the first light client
    
  ``` 
        log_level = "info"
        http_server_host = "127.0.0.1"
        http_server_port = "8000"
        libp2p_seed = 1
        libp2p_port = "38000"
        full_node_ws = ["ws://127.0.0.1:9944"]
        app_id = 1
        confidence = 92.0
        avail_path = "avail_path_2"
        prometheus_port = 9525
        bootstraps = [["12D3KooWBbKnhLfDBuzzN1RzeKHBoCnKK9E1nf1Vec3suhJYAEua", "/ip4/127.0.0.1/tcp/38000"]]
  
  ```
    
  
* run the second light-client with this configuration

  ``` cargo run -- -c config2.yaml ```
  
### Installation for Avail kate testnet

### Required nodes to run

* run the Avail light client simply with the following command:
  
  ``` curl --proto '=https' --tlsv1.2 -sSf https://raw.githubusercontent.com/availproject/availup/main/availup.sh | sh ```
  
  or with ``` wget ```:
  
  ``` wget --https-only --secure-protocol=TLSv1_2 -O - https://raw.githubusercontent.com/availproject/availup/main/availup.sh | sh ```


## Building your soverign rollup

Now that you have a da node and light client running, we are ready to build and run our Cosmos-SDK blockchain (here we have taken gm application)

* go to the root directory and install rollkit by adding the following lines to go.mod

    ``` 
    replace github.com/cosmos/cosmos-sdk => github.com/rollkit/cosmos-sdk v0.46.13-rollkit-v0.9.0-no-fraud-proofs

    replace github.com/centrifuge/go-substrate-rpc-client/v4 => github.com/availproject/go-substrate-rpc-client/v4 v4.0.12-avail-1.6.2-rc2 
    ```

    and run 
    ```
    go mod tidy

    ```
* create a config.json file
    * testnet environment [config](https://gist.github.com/chandiniv1/a844141dc9dda1d531da0eede69072a6)
    * local development environment [config](https://gist.github.com/chandiniv1/bc457ad6d36ec0c3ed5cc04314f9e163)

* 
    
    create one script file (init-local.sh) in root folder

    ``` 
    touch init-local.sh

    ```

*   add the following script to the script file (init-local.sh) or you can get the script from [here](https://gist.github.com/chandiniv1/27397b93e08e2c40e7e1b746f13e5d7b)


    ```
    #!/bin/sh

    # set variables for the chain
    VALIDATOR_NAME=validator1
    CHAIN_ID=gm
    KEY_NAME=gm-key-1
    KEY_2_NAME=gm-key-2
    CHAINFLAG="--chain-id ${CHAIN_ID}"
    TOKEN_AMOUNT="10000000000000000000000000stake"
    STAKING_AMOUNT="1000000000stake"

    # create a random Namespace ID for your rollup to post blocks to
    NAMESPACE_ID=$(openssl rand -hex 10)
    echo $NAMESPACE_ID

    DA_BLOCK_HEIGHT=$(curl http://localhost:8000/v1/latest_block | jq -r '.latest_block')

    echo $DA_BLOCK_HEIGHT

    # build the chain with Rollkit
    ignite chain build

    # build the application with Rollkit
    #make install

    # reset any existing genesis/chain data
    gmd tendermint unsafe-reset-all

    # initialize the validator with the chain ID you set
    gmd init $VALIDATOR_NAME --chain-id $CHAIN_ID

    # add keys for key 1 and key 2 to keyring-backend test
    gmd keys add $KEY_NAME --keyring-backend test
    gmd keys add $KEY_2_NAME --keyring-backend test

    # add these as genesis accounts
    gmd add-genesis-account $KEY_NAME $TOKEN_AMOUNT --keyring-backend test
    gmd add-genesis-account $KEY_2_NAME $TOKEN_AMOUNT --keyring-backend test

    # set the staking amounts in the genesis transaction
    gmd gentx $KEY_NAME $STAKING_AMOUNT --chain-id $CHAIN_ID --keyring-backend test

    # collect genesis transactions
    gmd collect-gentxs

    # start the chain
    gmd start --rollkit.aggregator true --rollkit.da_layer avail --rollkit.da_config "$(cat config.json)" --rollkit.namespace_id $NAMESPACE_ID --rollkit.da_start_height $DA_BLOCK_HEIGHT --api.enable --api.enabled-unsafe-cors

    ```
* run the rollup chain 

    ```
    bash init-local.sh

    ```




























