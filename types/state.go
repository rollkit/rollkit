package types

import (
	"fmt"
	"time"

	cmcrypto "github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmstate "github.com/cometbft/cometbft/proto/tendermint/state"
	cmversion "github.com/cometbft/cometbft/proto/tendermint/version"
	"github.com/cometbft/cometbft/types"
	"github.com/cometbft/cometbft/version"
	"github.com/rollkit/rollkit/config"
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

	// Merkle root of the results from executing prev block
	LastResultsHash Hash

	// the latest AppHash we've received from calling abci.Commit()
	AppHash Hash

	// In the MVP implementation, there will be only one Validator
	Validators                  *types.ValidatorSet
	NextValidators              *types.ValidatorSet
	LastValidators              *types.ValidatorSet
	LastHeightValidatorsChanged int64
}

// NewFromGenesisDoc reads blockchain State from genesis.
func NewFromGenesisDoc(genDoc *config.GenesisDoc) (State, error) {
	var validatorSet, nextValidatorSet *types.ValidatorSet
	if genDoc.ProposerAddress == nil {
		validatorSet = types.NewValidatorSet(nil)
		nextValidatorSet = types.NewValidatorSet(nil)
	} else {
		validators := make([]*types.Validator, 1)

		var pubKey cmcrypto.PubKey
		if len(genDoc.ProposerAddress) == ed25519.PubKeySize {
			pubKey = ed25519.PubKey(genDoc.ProposerAddress)
		} else {
			return State{}, fmt.Errorf("invalid proposer public key size: expected %d, got %d",
				ed25519.PubKeySize, len(genDoc.ProposerAddress))
		}

		validators[0] = types.NewValidator(pubKey, 1)

		validatorSet = types.NewValidatorSet(validators)
		nextValidatorSet = types.NewValidatorSet(validators).CopyIncrementProposerPriority(1)
	}

	s := State{
		Version:       InitStateVersion,
		ChainID:       genDoc.ChainID,
		InitialHeight: genDoc.InitialHeight,

		DAHeight: 1,

		LastBlockHeight: genDoc.InitialHeight - 1,
		LastBlockID:     types.BlockID{},
		LastBlockTime:   genDoc.GenesisTime,

		NextValidators:              nextValidatorSet,
		Validators:                  validatorSet,
		LastValidators:              validatorSet,
		LastHeightValidatorsChanged: int64(genDoc.InitialHeight),
	}

	return s, nil
}
