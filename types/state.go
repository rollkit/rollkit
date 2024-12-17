package types

import (
	"time"

	// TODO(tzdybal): copy to local project?

	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	"github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
)

// InitStateVersion sets the Consensus.Block and Software versions,
// but leaves the Consensus.App version blank.
// The Consensus.App version will be set during the Handshake, once
// we hear from the app what protocol version it is running.
var InitStateVersion = cmstate.Version{
	Consensus: cmversion.Consensus{
		Block: version.BlockProtocol,
		App:   0,
	},
	Software: version.TMCoreSemVer,
}

// State contains information about current state of the blockchain.
type State struct {
	Version cmstate.Version

	// immutable
	ChainID       string
	InitialHeight uint64 // should be 1, not 0, when starting from height 1

	// LastBlockHeight=0 at genesis (ie. block(H=0) does not exist)
	LastBlockHeight uint64
	LastBlockID     types.BlockID
	LastBlockTime   time.Time

	// DAHeight identifies DA block containing the latest applied Rollkit block.
	DAHeight uint64

	// Consensus parameters used for validating blocks.
	// Changes returned by EndBlock and updated after Commit.
	ConsensusParams                  cmproto.ConsensusParams
	LastHeightConsensusParamsChanged uint64

	// Merkle root of the results from executing prev block
	LastResultsHash Hash

	// the latest AppHash we've received from calling abci.Commit()
	AppHash Hash

	// In the MVP implementation, there will be only one Validator
	Validators                  *ValidatorSet
	NextValidators              *ValidatorSet
	LastValidators              *ValidatorSet
	LastHeightValidatorsChanged int64
}

// NewFromGenesisDoc reads blockchain State from genesis.
func NewFromGenesisDoc(genDoc *GenesisDoc) (State, error) {
	var validatorSet, nextValidatorSet *ValidatorSet
	if genDoc.Validators == nil {
		validatorSet = NewValidatorSet(nil)
		nextValidatorSet = NewValidatorSet(nil)
	} else {
		validators := make([]*Validator, len(genDoc.Validators))
		for i, val := range genDoc.Validators {
			validators[i] = &Validator{
				Address:     val.Address,
				PubKey:      val.PubKey,
				VotingPower: val.Power,
			}
		}
		validatorSet = NewValidatorSet(validators)
		nextValidatorSet = validatorSet.CopyIncrementProposerPriority(1)
	}

	s := State{
		Version:       InitStateVersion,
		ChainID:       genDoc.ChainID,
		InitialHeight: uint64(genDoc.InitialHeight),

		DAHeight: 1,

		LastBlockHeight: uint64(genDoc.InitialHeight) - 1,
		LastBlockID:     types.BlockID{},
		LastBlockTime:   genDoc.GenesisTime,

		NextValidators:              nextValidatorSet,
		Validators:                  validatorSet,
		LastValidators:              validatorSet,
		LastHeightValidatorsChanged: genDoc.InitialHeight,

		ConsensusParams: cmproto.ConsensusParams{
			Block: &cmproto.BlockParams{
				MaxBytes: genDoc.ConsensusParams.Block.MaxBytes,
				MaxGas:   genDoc.ConsensusParams.Block.MaxGas,
			},
			Evidence: &cmproto.EvidenceParams{
				MaxAgeNumBlocks: genDoc.ConsensusParams.Evidence.MaxAgeNumBlocks,
				MaxAgeDuration:  genDoc.ConsensusParams.Evidence.MaxAgeDuration,
				MaxBytes:        genDoc.ConsensusParams.Evidence.MaxBytes,
			},
			Validator: &cmproto.ValidatorParams{
				PubKeyTypes: genDoc.ConsensusParams.Validator.PubKeyTypes,
			},
			Version: &cmproto.VersionParams{
				App: genDoc.ConsensusParams.Version.App,
			},
			Abci: &cmproto.ABCIParams{
				VoteExtensionsEnableHeight: genDoc.ConsensusParams.ABCI.VoteExtensionsEnableHeight,
			},
		},
		LastHeightConsensusParamsChanged: uint64(genDoc.InitialHeight),
	}
	s.AppHash = genDoc.AppHash.Bytes()

	return s, nil
}

// CopyIncrementProposerPriority creates a copy of the validator set and increments the
// proposer priority n times.
func (valSet *ValidatorSet) CopyIncrementProposerPriority(n int) *ValidatorSet {
	// Create a copy of the validator set
	validators := make([]*Validator, len(valSet.Validators))
	for i, val := range valSet.Validators {
		validators[i] = &Validator{
			Address:     val.Address,
			PubKey:      val.PubKey,
			VotingPower: val.VotingPower,
		}
	}

	newValSet := NewValidatorSet(validators)

	// Increment the proposer priority n times
	for i := 0; i < n; i++ {
		newValSet.IncrementProposerPriority()
	}

	return newValSet
}

// IncrementProposerPriority increments the proposer priority for the validator set.
// In the MVP with a single validator, this is a no-op.
func (valSet *ValidatorSet) IncrementProposerPriority() {
	// For MVP with single validator, this is a no-op
	// TODO: Implement proper proposer selection algorithm when multiple validators are supported
}
