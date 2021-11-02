#!/bin/sh

CHAINID="test"
GENACCT="validator"

# Build genesis file incl account for passed address
COINS="10000000000stake,100000000000token"
rollupd init $CHAINID --chain-id $CHAINID 
rollupd keys add $GENACCT --keyring-backend="test"
rollupd add-genesis-account $(rollupd keys show $GENACCT -a --keyring-backend="test") $COINS
rollupd gentx $GENACCT 5000000000stake --keyring-backend="test" --chain-id $CHAINID
rollupd collect-gentxs

# Set proper defaults and change ports
sed -i 's#"tcp://127.0.0.1:26657"#"tcp://0.0.0.0:26657"#g' ~/.rollup/config/config.toml
sed -i 's/timeout_commit = "5s"/timeout_commit = "1s"/g' ~/.rollup/config/config.toml
sed -i 's/timeout_propose = "3s"/timeout_propose = "1s"/g' ~/.rollup/config/config.toml
sed -i 's/index_all_keys = false/index_all_keys = true/g' ~/.rollup/config/config.toml

