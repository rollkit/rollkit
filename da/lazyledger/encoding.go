package lazyledger

import (
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/lazyledger/lazyledger-app/app/params"
)

// RegisterAccountInterface registers the authtypes.AccountI interface to the
// interface registery in the provided encoding config
func RegisterAccountInterface(conf params.EncodingConfig) params.EncodingConfig {
	conf.InterfaceRegistry.RegisterInterface(
		"cosmos.auth.v1beta1.BaseAccount",
		(*authtypes.AccountI)(nil),
	)
	conf.InterfaceRegistry.RegisterImplementations(
		(*authtypes.AccountI)(nil),
		&authtypes.BaseAccount{},
	)
	return conf
}
