package types

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"
)

// InitStateVersion sets the Consensus.Block and Software versions,
// but leaves the Consensus.App version blank.
// The Consensus.App version will be set during the Handshake, once
// we hear from the app what protocol version it is running.
var InitStateVersion = Version{
	Block: 11, // Block version is set to 11, to be compatible with CometBFT blocks for IBC.
	App:   0,
}

// ErrUnexpectedProposer is returned when a block was signed by a proposer
// different from the proposer expected by the current state.
var ErrUnexpectedProposer = errors.New("unexpected proposer")

var (
	// ErrInvalidChainID is returned when a header belongs to a different chain than the current state.
	ErrInvalidChainID = errors.New("invalid chain ID")

	// ErrInvalidBlockHeight is returned when a header is not the next height after the current state.
	ErrInvalidBlockHeight = errors.New("invalid block height")

	// ErrInvalidBlockTime is returned when a header time is earlier than the current state's block time.
	ErrInvalidBlockTime = errors.New("invalid block time")

	// ErrInvalidLastHeaderHash is returned when a header does not link to the current state's last header.
	ErrInvalidLastHeaderHash = errors.New("invalid last header hash")

	// ErrInvalidLastAppHash is returned when a header does not reference the current state's app hash.
	ErrInvalidLastAppHash = errors.New("invalid last app hash")
)

// State contains information about the current state of the blockchain.
type State struct {
	Version Version

	// immutable
	ChainID       string
	InitialHeight uint64

	// LastBlockHeight=0 at genesis (ie. block(H=0) does not exist)
	LastBlockHeight uint64
	LastBlockTime   time.Time

	// LastHeaderHash is the hash of the header of the last block
	LastHeaderHash Hash

	// DAHeight identifies DA block containing the latest applied Evolve block for a syncing node.
	// In the case of an aggregator, this corresponds as the last fetched DA block height for forced included transactions.
	DAHeight uint64

	// the latest AppHash we've received from calling abci.Commit()
	AppHash []byte

	// NextProposerAddress is the proposer expected to sign LastBlockHeight+1.
	// It is initialized from genesis and then updated from execution results.
	NextProposerAddress []byte
}

func (s *State) NextState(header Header, stateRoot []byte, nextProposerAddress []byte) (State, error) {
	height := header.Height()
	nextProposer := s.NextProposerAddress
	if len(nextProposerAddress) > 0 {
		nextProposer = nextProposerAddress
	}
	if len(nextProposer) == 0 {
		nextProposer = header.ProposerAddress
	}

	return State{
		Version:             s.Version,
		ChainID:             s.ChainID,
		InitialHeight:       s.InitialHeight,
		LastBlockHeight:     height,
		LastBlockTime:       header.Time(),
		AppHash:             stateRoot,
		LastHeaderHash:      header.Hash(),
		DAHeight:            s.DAHeight,
		NextProposerAddress: cloneBytes(nextProposer),
	}, nil
}

// AssertValidForNextState performs common validation of a header and data against the current state.
// It assumes any context-specific basic header checks and verifier setup have already been performed
func (s State) AssertValidForNextState(header *SignedHeader, data *Data) error {
	if err := s.AssertExpectedProposer(header); err != nil {
		return err
	}

	if err := s.AssertValidSequence(header); err != nil {
		return err
	}

	if err := Validate(header, data); err != nil {
		return fmt.Errorf("header-data validation failed: %w", err)
	}
	return nil
}

// AssertExpectedProposer checks that the header was signed by the proposer expected for the next block.
func (s State) AssertExpectedProposer(header *SignedHeader) error {
	if len(s.NextProposerAddress) > 0 && !bytes.Equal(header.ProposerAddress, s.NextProposerAddress) {
		return fmt.Errorf("%w - got: %x, want: %x", ErrUnexpectedProposer, header.ProposerAddress, s.NextProposerAddress)
	}
	return nil
}

var (
	basedSequencerTracking sync.Once
	lastHeaderHashErrCount = 0
)

// AssertValidSequence performs lightweight state-sequence validation for self-produced blocks.
func (s State) AssertValidSequence(header *SignedHeader) error {
	if header.ChainID() != s.ChainID {
		return fmt.Errorf("%w - got %s, want %s", ErrInvalidChainID, header.ChainID(), s.ChainID)
	}

	if len(s.LastHeaderHash) == 0 { // initial state
		return nil
	}

	if expdHeight := s.LastBlockHeight + 1; header.Height() != expdHeight {
		return fmt.Errorf("%w - got: %d, want: %d", ErrInvalidBlockHeight, header.Height(), expdHeight)
	}

	if headerTime := header.Time(); s.LastBlockTime.After(headerTime) {
		return fmt.Errorf("%w - got: %v, last: %v", ErrInvalidBlockTime, headerTime, s.LastBlockTime)
	}

	// Trick to support the switch from a based sequencer to a normal syncing node.
	// Based sequencers do not sign headers, meaning the last header hash will be different from the
	// newly derived header hash when a based sequencer has switched to a syncing node.
	if !bytes.Equal(header.LastHeaderHash, s.LastHeaderHash) {
		if lastHeaderHashErrCount == 1 {
			return fmt.Errorf("%w - got: %x, want: %x", ErrInvalidLastHeaderHash, header.LastHeaderHash, s.LastHeaderHash)
		}

		basedSequencerTracking.Do(func() {
			lastHeaderHashErrCount++
		})
	}

	if !bytes.Equal(header.AppHash, s.AppHash) {
		return fmt.Errorf("%w - got: %x, want: %x", ErrInvalidLastAppHash, header.AppHash, s.AppHash)
	}

	return nil
}
