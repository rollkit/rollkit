package header

import (
	"encoding"
	"time"
)

// Header abstracts all methods required to perform header sync.
// TODO: Ideally, this should be Header[H Header[H]], but GO does not support recursive type
// definitions (yet?)
type Header[H any] interface {
	// New creates new instance of a header.
	// It exists to overcome limitation of Go's type system.
	// See:
	//https://go.googlesource.com/proposal/+/refs/heads/master/design/43651-type-parameters.md#pointer-method-example
	New() H
	// IsZero reports whether Header is a zero value of it's concrete type.
	IsZero() bool
	// ChainID returns identifier of the chain.
	ChainID() string
	// Hash returns hash of a header.
	Hash() Hash
	// Height returns the height of a header.
	Height() uint64
	// LastHeader returns the hash of last header before this header (aka. previous header hash).
	LastHeader() Hash
	// Time returns time when header was created.
	Time() time.Time
	// Verify validates given untrusted Header against trusted Header.
	Verify(H) error
	// Validate performs stateless validation to check for missed/incorrect fields.
	Validate() error

	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

// New is a generic Header constructor.
func New[H Header[H]]() (h H) {
	return h.New()
}
