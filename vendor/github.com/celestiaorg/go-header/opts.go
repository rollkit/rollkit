package header

type HeadOption[H Header[H]] func(opts *HeadParams[H])

// HeadParams contains options to be used for Head interface methods
type HeadParams[H Header[H]] struct {
	// TrustedHead allows the caller of Head to specify a trusted header
	// against which the underlying implementation of Head can verify against.
	TrustedHead H
}

// WithTrustedHead sets the TrustedHead parameter to the given header.
func WithTrustedHead[H Header[H]](verified H) func(opts *HeadParams[H]) {
	return func(opts *HeadParams[H]) {
		opts.TrustedHead = verified
	}
}
