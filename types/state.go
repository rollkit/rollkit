package types

import (
	"fmt"
	"time"

	"github.com/cometbft/cometbft/types"
	pb "github.com/rollkit/rollkit/types/pb/rollkit/v1"
)

// InitStateVersion sets the Consensus.Block and Software versions,
// but leaves the Consensus.App version blank.
// The Consensus.App version will be set during the Handshake, once
// we hear from the app what protocol version it is running.
var InitStateVersion = pb.Version{
	Block: 1,
	App:   0,
}

// State contains information about current state of the blockchain.
type State struct {
	Version pb.Version

	// immutable
	ChainID       string
	InitialHeight uint64 // should be 1, not 0, when starting from height 1

	// LastBlockHeight=0 at genesis (ie. block(H=0) does not exist)
	LastBlockHeight uint64
	LastBlockTime   time.Time

	// DAHeight identifies DA block containing the latest applied Rollkit block.
	DAHeight uint64

	// Merkle root of the results from executing prev block
	LastResultsHash Hash

	// the latest AppHash we've received from calling abci.Commit()
	AppHash Hash
}

// NewFromGenesisDoc reads blockchain State from genesis.
func NewFromGenesisDoc(genDoc *types.GenesisDoc) (State, error) {
	err := genDoc.ValidateAndComplete()
	if err != nil {
		return State{}, fmt.Errorf("error in genesis doc: %w", err)
	}

	s := State{
		Version:       InitStateVersion,
		ChainID:       genDoc.ChainID,
		InitialHeight: uint64(genDoc.InitialHeight),

		DAHeight: 1,

		LastBlockHeight: uint64(genDoc.InitialHeight) - 1,
		LastBlockTime:   genDoc.GenesisTime,
	}
	s.AppHash = genDoc.AppHash.Bytes()

	return s, nil
}
