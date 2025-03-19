package block

import (
	"github.com/rollkit/rollkit/types"
)

// Event types
const (
	EventBlockCreated    = "block.created"
	EventBlockPublished  = "block.published"
	EventBlockDAIncluded = "block.da_included"
	EventHeaderRetrieved = "header.retrieved"
	EventDataRetrieved   = "data.retrieved"
	EventStateUpdated    = "state.updated"
	EventSequencerBatch  = "sequencer.batch"
	EventDAHeightChanged = "da.height_changed"
)

// BlockCreatedEvent is published when a new block is created
type BlockCreatedEvent struct {
	Header *types.SignedHeader
	Data   *types.Data
	Height uint64
}

// Type returns the event type
func (e BlockCreatedEvent) Type() string {
	return EventBlockCreated
}

// BlockPublishedEvent is published when a block is published to P2P network
type BlockPublishedEvent struct {
	Header *types.SignedHeader
	Data   *types.Data
	Height uint64
}

// Type returns the event type
func (e BlockPublishedEvent) Type() string {
	return EventBlockPublished
}

// BlockDAIncludedEvent is published when a block is included in DA layer
type BlockDAIncludedEvent struct {
	Header   *types.SignedHeader
	Height   uint64
	DAHeight uint64
}

// Type returns the event type
func (e BlockDAIncludedEvent) Type() string {
	return EventBlockDAIncluded
}

// HeaderRetrievedEvent is published when a header is retrieved from P2P or DA
type HeaderRetrievedEvent struct {
	Header   *types.SignedHeader
	DAHeight uint64
	Source   string // "p2p" or "da"
}

// Type returns the event type
func (e HeaderRetrievedEvent) Type() string {
	return EventHeaderRetrieved
}

// DataRetrievedEvent is published when data is retrieved from P2P or DA
type DataRetrievedEvent struct {
	Data     *types.Data
	DAHeight uint64
	Source   string // "p2p" or "da"
}

// Type returns the event type
func (e DataRetrievedEvent) Type() string {
	return EventDataRetrieved
}

// StateUpdatedEvent is published when the state is updated
type StateUpdatedEvent struct {
	State  types.State
	Height uint64
}

// Type returns the event type
func (e StateUpdatedEvent) Type() string {
	return EventStateUpdated
}

// SequencerBatchEvent is published when a new batch is received from sequencer
type SequencerBatchEvent struct {
	Batch *BatchWithTime
}

// Type returns the event type
func (e SequencerBatchEvent) Type() string {
	return EventSequencerBatch
}

// DAHeightChangedEvent is published when the DA height changes
type DAHeightChangedEvent struct {
	Height uint64
}

// Type returns the event type
func (e DAHeightChangedEvent) Type() string {
	return EventDAHeightChanged
}

// NewHeaderEvent is used to pass header and DA height to headerInCh
type NewHeaderEvent struct {
	Header   *types.SignedHeader
	DAHeight uint64
}

// NewDataEvent is used to pass header and DA height to headerInCh
type NewDataEvent struct {
	Data     *types.Data
	DAHeight uint64
}
