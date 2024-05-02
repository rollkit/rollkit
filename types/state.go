package types

import (
	"fmt"
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
	Validators *types.ValidatorSet
}

// NewFromGenesisDoc reads blockchain State from genesis.
func NewFromGenesisDoc(genDoc *types.GenesisDoc) (State, error) {
	err := genDoc.ValidateAndComplete()
	if err != nil {
		return State{}, fmt.Errorf("error in genesis doc: %w", err)
	}

	// if len(genDoc.Validators) != 1 {
	// 	return State{}, fmt.Errorf("must have exactly 1 validator (the centralized sequencer)")
	// }

	s := State{
		Version:       InitStateVersion,
		ChainID:       genDoc.ChainID,
		InitialHeight: uint64(genDoc.InitialHeight),

		DAHeight: 1,

		LastBlockHeight: uint64(genDoc.InitialHeight) - 1,
		LastBlockID:     types.BlockID{},
		LastBlockTime:   genDoc.GenesisTime,

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
		},
		LastHeightConsensusParamsChanged: uint64(genDoc.InitialHeight),
	}
	s.AppHash = genDoc.AppHash.Bytes()

	return s, nil
}
