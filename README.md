
# Avail-da

This package implements DataAvailabilityLayerClient interface

## Installation

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

#### 2. Avail light node


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
    2023-09-12T05:44:23.869487Z  INFO avail_light::network::event_loop: Local node is listening on "/ip4/172.17.0.1/tcp/37000"  ```

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
* run the second light-client with this configuration

    ``` cargo run -- -c config2.yaml ```

## Building your soverign rollup

Now that you have a da node and light nodes running, we are ready to build and run our Cosmos-SDK blockchain

* go to the root directory and install rollkit by adding the following lines to go.mod

    ``` 
    replace github.com/cosmos/cosmos-sdk => github.com/rollkit/cosmos-sdk v0.46.13-rollkit-v0.9.0-no-fraud-proofs

    replace github.com/tendermint/tendermint => github.com/rollkit/cometbft v0.0.0-20230524013049-75272ebaee38

    replace github.com/centrifuge/go-substrate-rpc-client/v4 => github.com/availproject/go-substrate-rpc-client/v4 v4.0.12-avail-1.6.2-rc2 
    ```

    and run 
    ```
    go mod tidy

    ```
* start your rollup 
    
    create one script file (init-local.sh) 

    ``` 
    touch init-local.sh

    ```
    add the following script to the script file (init-local.sh)

    ```
    #!/bin/sh

    # set variables for the chain
    VALIDATOR_NAME=validator1
    CHAIN_ID=gm
    KEY_NAME=gm-key
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
    regen tendermint unsafe-reset-all

    # initialize the validator with the chain ID you set
    regen init $VALIDATOR_NAME --chain-id $CHAIN_ID

    # add keys for key 1 and key 2 to keyring-backend test
    regen keys add $KEY_NAME --keyring-backend test
    regen keys add $KEY_2_NAME --keyring-backend test

    # add these as genesis accounts
    regen add-genesis-account $KEY_NAME $TOKEN_AMOUNT --keyring-backend test
    regen add-genesis-account $KEY_2_NAME $TOKEN_AMOUNT --keyring-backend test

    # set the staking amounts in the genesis transaction
    regen gentx $KEY_NAME $STAKING_AMOUNT --chain-id $CHAIN_ID --keyring-backend test

    # collect genesis transactions
    regen collect-gentxs

    # start the chain
    regen start --rollkit.aggregator true --rollkit.da_layer avail --rollkit.da_config='{"base_url":"http://localhost:8000/v1", "seed":"bottom drive obey lake curtain smoke basket hold race lonely fit walk//Alice","api_url":"ws://127.0.0.1:9944","app_data_url": "/appdata/%d?decode=true","app_id" : 1,"confidence":92}' --rollkit.namespace_id $NAMESPACE_ID --rollkit.da_start_height 70 --api.enable --api.enabled-unsafe-cors

    ```
    run the rollup chain 

    ```
    bash init-local.sh

    ```
* transactions

    Now we can test sending a transaction from one of our keys to the other. We can do that with the following command:

    ```
    regen tx bank send regen15rw6kf6sv4yy5t9xvqff3uglkvspylvz64wfc8 regen1jpqqy954kj0g5cdjyxlrhcd4ms08y8vm38qnsd 99stake --keyring-backend test --chain-id gm

    ```
    You'll be prompted to accept the transaction:

    ```
    auth info:
    fee:
    amount: []
    gas_limit: "200000"
    granter: ""
    payer: ""
    signer_infos: []
    tip: null
    extension_options: []
    body:
    memo:
    messages:
    - '@type': /cosmos. bank.v1betal.MsgSend
    amount:
    - amount: "99"
    denom: stake
    from_address: regen15rw6kf6sv4yy5t9xvqff3uglkvspylvz64wfc8
    to_address: regen1jpqqy954kj0g5cdjyxlrhcd4ms08y8vm38qnsd
    non_critical_extension_options: []
    timeout height: "0"
    signatures: []
    confirm transaction before signing and broadcasting [y/N]: y

    ```
    Type y if you'd like to confirm and sign the transaction. Then, you'll see the confirmation:

    ```
    code: 0
    codespace:
    data: ""
    events: []
    gas_used: "0"
    gas_wanted: "0"
    height: "0"
    info: ""
    logs: []
    raw_log: '[]'
    timestamp:
    tx: null
    txhash: AC190599B4413B41EE8F86DA34F92F14926094EAC804DDFC7A6F08571C236139
    
    ```
    we can query the transaction using ``` txhash ```

    ```
    regen q tx AC190599B4413B41EE8F86DA34F92F14926094EAC804DDFC7A6F08571C236139

    ```
    you will be prompted like :

    ```
    code: 0
    codespace:""
    data: 12260A242F636F736D6F732E62616E6B2E763162657461312E4D736753656E64526573706F6E7365
    events: 
    - attributes:
        - index: true 
          key: ZmV
          value: null
        - index: true
          key: ZmVLX3BheWVy
          value: cmVnZW4XNXJ3NmtmNnN2NH15NXQ5eHZxZmYzdWdsa3ZzcHlsdno2NHdmYzg=
        type: tx
    - attributes:
        - index: true
          key: YWNjX3NLCQ==
          value: cmVnZW4XNXJ3NmtmNnN2NH15NXQ5eHZxZmYzdWdsa3ZzcHlsdno2NHdmYzgvMg==
        type: tx
    - attributes:
        - index: true
          key: c2lnbmF0dXJl
          value: RG9hb1NFTVBwSGZXd2dlZzNUbE4zWm5DcUYzdlg5WkREdzMwQ0tPTVJh0E56cmh0aGwyZlM4cWJ6WS9sTzdXcXJvNThoTGFPc0dCUFZTWVNDVDMOVFE9PQ==
        type: tx
    - attributes:
        - index: true
          key: YWN0aW9u
          value: L2Nvc21vcy5iYW5rLnYxYmV0YTEuTXNnU2VuZA==
        type: message
    - attributes:
        - index: true
          key: c3BlbmRlcg==
          value: cmVnZW4XNXJ3NmtmNnN2NH15NXQ5eHZxZmYzdWdsa3ZzcHlsdno2NHdmYzg=
        - index: true
          key: YW1vdW50
          value: OTlzdGFrZQ==
        type: coin_spent
    - attributes:
        - index: true
          key: cmVjZWL2ZXI=
          value: cmVnZW4xanBxcXk5NTRгajBnNwNkanl4bHJoY2Q0bXMwOHk4dm0z0HFuc20=
        - index: true
          key: YW1vdw50
          value: OTlzdGFrZQ==
    Log: ""
    msg_index: 0 
    raw_log:'[{"msg_index":0,"events":[{"type":"coin_received","attributes":[{"key":"receiver","value":"regen1jpqqy954kj0g5cdjyxlrhcd4ms@8y8vm38qnsd"},{"key":"amount","value":"99stake"}]},{"type":"coin_spent","attributes":[{"key":"spender","value":"regen15rw6kf6sv4yy5t9x vqff3uglkvspylvz64wfc8"},{"key":"amount", "value":"99stake"}]},{"type":"message","attributes":[{"key":"action","value":"/cosmos.bank.v1 betal.MsgSend"},{"key": "sender","value":"regen15rw6kf6sv4yy5t9xvqff3uglkvspylvz64wfc8"},{"key":"module","value":"bank"}]},{"type":"transfer","attributes":[{"key":"recipient","value":"regen1jpqqy954kj0g5cdjyxlrhcd4ms08y8vm38qnsd"},{"key": "sender", "value":"regen15rw6kf6sv4yy5t9xvqff3uglkvspylvz64wfc8"},{"key":"amount", "value":"99stake"))}]}]·
    timestamp: "2023-08-10T07:21:54Z"
    tx:
        '@type': /cosmos.tx.vlbetal.Tx
      auth_info:
        fee:
            amount: []
            gas_limit: "200000"
            granter: ""
            payer: ""
        signer_infos:
        - mode_info:
            single:
            mode: SIGN_MODE_DIRECT
          public_key:
            "@type': /cosmos.crypto.secp256k1.PubKey
            key: Avik+Bm1Yzda9iKywoT9XdLHt6WJ/nxhEZUYMAZMhdGP 
          sequence: "2"
        tip: null
      body:
        extension_options: []
        memo: ""
        messages:
        - "@type': /cosmos.bank.vlbetal.MsgSend
          amount:
            - amount: "99"
              denom: stake
            from_address: regen15rw6kf6sv4yy5t9xvqff3uglkvspylvz64wfc8
            to_address: regen1jpqqy954kj0g5cdjyxl rhcd4ms08y8vm38qnsd
      non_critical_extension_options: []
      timeout_height: "0"
    signatures:
    - DoaoSEMPpHfwwgeg3TLN3ZnCqF3vX9ZDDw30CKOMRa8Nzrhthl2f58qbzY/107wqro58hLa0sGBPVSYSCT34TQ==
    txhash: AC190599B4413B41EE8F86DA34F92F14926094EAC804DDFC7A6F08571C236139
    ```



















