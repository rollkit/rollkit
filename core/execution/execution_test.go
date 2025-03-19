package execution_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/rollkit/rollkit/core/execution"
	"github.com/stretchr/testify/require"
)

// CosmosPreGenesis represents the pre-genesis data for a Cosmos-based chain
type CosmosPreGenesis struct {
	ChainIdentifier    string
	Accounts           []CosmosAccount
	ModuleAccounts     []ModuleAccount
	StakingParams      StakingParams
	GovParams          GovParams
	BankParams         BankParams
	DistributionParams DistributionParams
}

// CosmosAccount represents an account in the Cosmos ecosystem
type CosmosAccount struct {
	Address     string
	Balance     []CosmosCoin
	PubKey      string
	AccountType string // can be "base", "vesting", "module"
}

// ModuleAccount represents a module account in Cosmos SDK
type ModuleAccount struct {
	Name        string
	Permissions []string // e.g., ["minter", "burner", "staking"]
}

// CosmosCoin represents a token denomination and amount in Cosmos
type CosmosCoin struct {
	Denom  string
	Amount uint64
}

// StakingParams defines the parameters for the staking module
type StakingParams struct {
	UnbondingTime     time.Duration
	MaxValidators     uint32
	BondDenom         string
	MinSelfDelegation uint64
}

// GovParams defines the parameters for the governance module
type GovParams struct {
	VotingPeriod     time.Duration
	QuorumPercentage float64
	ThresholdPercent float64
	MinDepositAmount uint64
	MinDepositDenom  string
}

// BankParams defines the parameters for the bank module
type BankParams struct {
	SendEnabled        bool
	DefaultSendEnabled bool
}

// DistributionParams defines the parameters for the distribution module
type DistributionParams struct {
	CommunityTax        float64
	BaseProposerReward  float64
	BonusProposerReward float64
}

// EVMPreGenesis represents the pre-genesis data for an EVM-based chain
type EVMPreGenesis struct {
	ChainIdentifier string
	Alloc           map[string]EVMAccount
	Config          EVMChainConfig
	Consensus       ConsensusConfig
	GasLimit        uint64
}

// EVMAccount represents an account in the EVM
type EVMAccount struct {
	Balance    string            // Balance in wei
	Code       string            // Contract code if this is a contract account
	Storage    map[string]string // Contract storage
	Nonce      uint64            // Account nonce
	PrivateKey string            // Optional, for testing purposes only
}

// EVMChainConfig contains the configuration for various network upgrades
type EVMChainConfig struct {
	ChainID             uint64
	HomesteadBlock      uint64
	EIP150Block         uint64
	EIP155Block         uint64
	EIP158Block         uint64
	ByzantiumBlock      uint64
	ConstantinopleBlock uint64
	PetersburgBlock     uint64
	IstanbulBlock       uint64
	MuirGlacierBlock    uint64
	BerlinBlock         uint64
	LondonBlock         uint64
}

// ConsensusConfig defines the consensus engine configuration
type ConsensusConfig struct {
	Engine       string        // Supported values: "ethash", "clique"
	CliqueConfig *CliqueConfig // Configuration for Clique consensus
	EthashConfig *EthashConfig // Configuration for Ethash consensus
}

// CliqueConfig contains configuration for the Clique consensus engine
type CliqueConfig struct {
	Period uint64 // Number of seconds between blocks
	Epoch  uint64 // Number of blocks after which to reset votes
}

// EthashConfig contains configuration for the Ethash consensus engine
type EthashConfig struct {
	FixedDifficulty *uint64 // Optional fixed difficulty for testing
}

// Implement CommonPreGenesis interface for CosmosPreGenesis
func (cpg CosmosPreGenesis) ChainID() string {
	return cpg.ChainIdentifier
}

// Implement CommonPreGenesis interface for EVMPreGenesis
func (epg EVMPreGenesis) ChainID() string {
	return epg.ChainIdentifier
}

// CosmosGenesis represents the genesis state for a Cosmos-based chain
type CosmosGenesis struct {
	genesisState []byte
}

// Bytes returns the genesis state as a byte slice
func (cg CosmosGenesis) Bytes() []byte {
	return cg.genesisState
}

// Validate performs validation on the genesis state
func (cg CosmosGenesis) Validate() error {
	// In a real implementation, we would validate the genesis state
	return nil
}

// EVMGenesis represents the genesis state for an EVM-based chain
type EVMGenesis struct {
	genesisState []byte
}

// Bytes returns the genesis state as a byte slice
func (eg EVMGenesis) Bytes() []byte {
	return eg.genesisState
}

// Validate performs validation on the genesis state
func (eg EVMGenesis) Validate() error {
	// In a real implementation, we would validate the genesis state
	return nil
}

// CosmosGenesisProvider implements GenesisProvider for Cosmos-based chains
type CosmosGenesisProvider struct{}

// BuildGenesis creates a new Genesis state from CosmosPreGenesis data
func (cgp CosmosGenesisProvider) BuildGenesis(preGenesis CosmosPreGenesis) (CosmosGenesis, error) {
	// In a real implementation, this would create the actual genesis state
	// Here we just marshal the pre-genesis data as an example
	state, err := json.Marshal(preGenesis)
	if err != nil {
		return CosmosGenesis{}, err
	}
	return CosmosGenesis{genesisState: state}, nil
}

// EVMGenesisProvider implements GenesisProvider for EVM-based chains
type EVMGenesisProvider struct{}

// BuildGenesis creates a new Genesis state from EVMPreGenesis data
func (egp EVMGenesisProvider) BuildGenesis(preGenesis EVMPreGenesis) (EVMGenesis, error) {
	// In a real implementation, this would create the actual EVM genesis state
	// Here we just marshal the pre-genesis data as an example
	state, err := json.Marshal(preGenesis)
	if err != nil {
		return EVMGenesis{}, err
	}
	return EVMGenesis{genesisState: state}, nil
}

// buildGenesisWithProvider is a generic function that demonstrates how the GenesisProvider interface
// can work with any implementation that meets the requirements
func buildGenesisWithProvider[P execution.CommonPreGenesis, G execution.Genesis](
	t *testing.T,
	provider execution.GenesisProvider[P, G],
	preGenesis P,
	expectedChainID string,
) {
	t.Helper()

	genesis, err := provider.BuildGenesis(preGenesis)
	require.NoError(t, err)
	require.NotNil(t, genesis.Bytes())

	// Verify that the ChainID matches
	require.Equal(t, expectedChainID, preGenesis.ChainID())

	// Verify that the genesis state is valid
	require.NoError(t, genesis.Validate())
}

func TestGenericGenesisProvider(t *testing.T) {
	t.Run("Generic Provider works with Cosmos Implementation", func(t *testing.T) {
		var provider execution.GenesisProvider[CosmosPreGenesis, CosmosGenesis] = CosmosGenesisProvider{}

		preGenesis := CosmosPreGenesis{
			ChainIdentifier: "test-cosmos-1",
			Accounts: []CosmosAccount{
				{
					Address: "cosmos1abc...",
					Balance: []CosmosCoin{
						{Denom: "stake", Amount: 1000000},
						{Denom: "atom", Amount: 5000000},
					},
					PubKey:      "cosmospub1...",
					AccountType: "base",
				},
			},
			ModuleAccounts: []ModuleAccount{
				{
					Name:        "distribution",
					Permissions: []string{"basic"},
				},
			},
			StakingParams: StakingParams{
				UnbondingTime:     time.Hour * 24 * 21, // 21 days
				MaxValidators:     100,
				BondDenom:         "stake",
				MinSelfDelegation: 1000000,
			},
			GovParams: GovParams{
				VotingPeriod:     time.Hour * 24 * 14, // 14 days
				QuorumPercentage: 0.334,               // 33.4%
				ThresholdPercent: 0.5,                 // 50%
				MinDepositAmount: 10000000,            // 10 ATOM
				MinDepositDenom:  "stake",
			},
			BankParams: BankParams{
				SendEnabled:        true,
				DefaultSendEnabled: true,
			},
			DistributionParams: DistributionParams{
				CommunityTax:        0.02, // 2%
				BaseProposerReward:  0.01, // 1%
				BonusProposerReward: 0.04, // 4%
			},
		}

		buildGenesisWithProvider(t, provider, preGenesis, "test-cosmos-1")
	})

	t.Run("Generic Provider works with EVM Implementation", func(t *testing.T) {
		var provider execution.GenesisProvider[EVMPreGenesis, EVMGenesis] = EVMGenesisProvider{}

		preGenesis := EVMPreGenesis{
			ChainIdentifier: "test-evm-1",
			Alloc: map[string]EVMAccount{
				"0x123...": {
					Balance: "1000000000000000000", // 1 ETH in wei
					Code:    "",
					Storage: map[string]string{
						"0x0000000000000000000000000000000000000000000000000000000000000001": "0x0000000000000000000000000000000000000000000000000000000000000001",
					},
					Nonce: 0,
				},
			},
			Config: EVMChainConfig{
				ChainID:             1337,
				HomesteadBlock:      0,
				EIP150Block:         0,
				EIP155Block:         0,
				EIP158Block:         0,
				ByzantiumBlock:      0,
				ConstantinopleBlock: 0,
				PetersburgBlock:     0,
				IstanbulBlock:       0,
				MuirGlacierBlock:    0,
				BerlinBlock:         0,
				LondonBlock:         0,
			},
			Consensus: ConsensusConfig{
				Engine: "clique",
				CliqueConfig: &CliqueConfig{
					Period: 15, // 15 second blocks
					Epoch:  30000,
				},
			},
			GasLimit: 8000000,
		}

		buildGenesisWithProvider(t, provider, preGenesis, "test-evm-1")
	})
}
