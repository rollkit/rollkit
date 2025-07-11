package evm

const (
	FlagEvmEthURL       = "evm.eth-url"
	FlagEvmEngineURL    = "evm.engine-url"
	FlagEvmJWTSecret    = "evm.jwt-secret"    //nolint:gosec // G101: This is a flag name, not a hardcoded credential
	FlagEvmGenesisHash  = "evm.genesis-hash"
	FlagEvmFeeRecipient = "evm.fee-recipient"
)
