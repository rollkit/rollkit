package types

import (
	"errors"
	"time"

	"github.com/rollkit/rollkit/pkg/genesis"
)

// InitStateVersion sets the Consensus.Block and Software versions,
// but leaves the Consensus.App version blank.
// The Consensus.App version will be set during the Handshake, once
// we hear from the app what protocol version it is running.
var InitStateVersion = Version{
	Block: 1,
	App:   0,
}

// State contains information about current state of the blockchain.
type State struct {
	Version Version

	// immutable
	ChainID       string
	InitialHeight uint64

	// LastBlockHeight=0 at genesis (ie. block(H=0) does not exist)
	LastBlockHeight uint64
	LastBlockTime   time.Time

	// DAHeight identifies DA block containing the latest applied Rollkit block.
	DAHeight uint64

	// Merkle root of the results from executing prev block
	LastResultsHash Hash

	// the latest AppHash we've received from calling abci.Commit()
	AppHash []byte
}

// NewFromGenesisDoc reads blockchain State from genesis.
func NewFromGenesisDoc(genDoc genesis.Genesis) (State, error) {
	if genDoc.InitialHeight == 0 {
		return State{}, errors.New("initial height must be 1 when starting a new app")
	}
	s := State{
		Version:       InitStateVersion,
		ChainID:       genDoc.ChainID,
		InitialHeight: genDoc.InitialHeight,

		DAHeight: 1,

		LastBlockHeight: genDoc.InitialHeight - 1,
		LastBlockTime:   genDoc.GenesisDAStartTime,
	}

	return s, nil
}

func (s *State) NextState(header *SignedHeader, stateRoot []byte) (State, error) {
	height := header.Height()

	return State{
		Version:         s.Version,
		ChainID:         s.ChainID,
		InitialHeight:   s.InitialHeight,
		LastBlockHeight: height,
		LastBlockTime:   header.Time(),
		AppHash:         stateRoot,
		DAHeight:        s.DAHeight,
	}, nil
}
