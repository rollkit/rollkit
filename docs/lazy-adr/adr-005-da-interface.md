# ADR 005: Data Availability Client Interface

## Changelog

- 2021.04.30: Initial draft
- 2021.06.03: Init method added

## Context

Optimint requires data availability layer. Different implementations are expected.

## Alternative Approaches

> This section contains information around alternative options that are considered before making a decision. It should contain a explanation on why the alternative approach(es) were not chosen.

## Decision

Defined interface should be very generic.
Interface should consist of 4 methods: `Init`, `Start`, `Stop`, `SubmitBlock`.
All the details are implementation-specific.

## Detailed Design

Definition of interface:
```go
type DataAvailabilityLayerClient interface {
	// Init is called once to allow DA client to read configuration and initialize resources.
	Init(config []byte, logger log.Logger) error

	Start() error
	Stop() error

	// SubmitBlock submits the passed in block to the DA layer.
	// This should create a transaction which (potentially)
	// triggers a state transition in the DA layer.
	SubmitBlock(block *types.Block) ResultSubmitBlock
}

// TODO define an enum of different non-happy-path cases
// that might need to be handled by Optimint independent of
// the underlying DA chain.
type StatusCode uint64

const (
	StatusSuccess StatusCode = iota
	StatusTimeout
	StatusError
)
```
>

## Status

Implemented

## Consequences

> This section describes the consequences, after applying the decision. All consequences should be summarized here, not just the "positive" ones.

### Positive

### Negative

### Neutral

## References

> Are there any relevant PR comments, issues that led up to this, or articles referenced for why we made the given design choice? If so link them here!

