package state

import (
	"errors"

	"github.com/tendermint/tendermint/proxy"
)

// Rollkit supports receiving soft-confirmations.
// This means the ACBI app's state must be rewindable,
// in the event of a re-org caused by the observation of a block
// that wins the fork-choice rule prior to finalization of a chain-segment in DA
// we store the LatestFinalizedHeight to reject blocks that would override finality
// we provide a RevertSoftToHeight function to support querying re-orgable state
type RewindableAppConnConsensus interface {
	proxy.AppConnConsensus
	// ABCI apps supporting soft confirmations should have this feature so the rollups can be queried prior to finality
	RevertSoftToHeight(int64) error
	LatestFinalizedHeight() int64
}

// This function takes a normal non-rewindable ABCI connection and wraps it
func RewindableFromStandard(app proxy.AppConnConsensus) rewindableAppConn {
	return rewindableAppConn{
		app,
		0,
		false,
	}
}

var _ RewindableAppConnConsensus = rewindableAppConn{}

type rewindableAppConn struct {
	proxy.AppConnConsensus
	latestFinalizedHeight int64
	rewindable            bool
}

func (r rewindableAppConn) RevertSoftToHeight(height int64) error {
	return errors.New("not implemented")
}
func (r rewindableAppConn) LatestFinalizedHeight() int64 {
	return r.latestFinalizedHeight
}
