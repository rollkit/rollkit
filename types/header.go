package types

import (
	"bytes"
	"encoding"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// TODO: remove Hash and H definition after importing go-header package
type Hash []byte

func (h Hash) String() string {
	return strings.ToUpper(hex.EncodeToString(h))
}

func (h Hash) MarshalJSON() ([]byte, error) {
	s := strings.ToUpper(hex.EncodeToString(h))
	jbz := make([]byte, len(s)+2)
	jbz[0] = '"'
	copy(jbz[1:], s)
	jbz[len(jbz)-1] = '"'
	return jbz, nil
}

func (h *Hash) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' {
		return fmt.Errorf("invalid hex string: %s", data)
	}
	bz2, err := hex.DecodeString(string(data[1 : len(data)-1]))
	if err != nil {
		return err
	}
	*h = bz2
	return nil
}

// Header abstracts all methods required to perform header sync.
type H interface {
	// New creates new instance of a header.
	New() H
	// Hash returns hash of a header.
	Hash() [32]byte
	// Number returns the height of a header.
	Number() int64
	// LastHeader returns the hash of last header before this header (aka. previous header hash).
	LastHeader() [32]byte
	// Timestamp returns time when header was created.
	Timestamp() time.Time
	// IsRecent checks if header is recent against the given blockTime.
	IsRecent(duration time.Duration) bool
	// IsExpired checks if header is expired against trusting period.
	IsExpired() bool
	// VerifyAdjacent validates adjacent untrusted header against trusted header.
	VerifyAdjacent(H) error
	// VerifyNonAdjacent validates non-adjacent untrusted header against trusted header.
	VerifyNonAdjacent(H) error
	// Verify performs basic verification of untrusted header.
	Verify(H) error
	// Validate performs basic validation to check for missed/incorrect fields.
	Validate() error

	encoding.BinaryMarshaler
	encoding.BinaryUnmarshaler
}

var _ H = (*Header)(nil)

// New creates new instance of a header.
func (h *Header) New() H {
	return &Header{}
}

// Number returns the height of a header.
func (h *Header) Number() int64 {
	return int64(h.Height)
}

// Timestamp returns time when header was created.
func (h *Header) Timestamp() time.Time {
	return time.Unix(int64(h.Time), 0)
}

// LastHeader returns the hash of last header before this header (aka. previous header hash).
func (h *Header) LastHeader() [32]byte {
	return h.LastHeaderHash
}

// IsRecent checks if header is recent against the given blockTime.
func (h *Header) IsRecent(blockTime time.Duration) bool {
	return time.Since(time.Unix(int64(h.Time), 0)) <= blockTime
}

// TODO(@Wondertan): We should request TrustingPeriod from the network's state params or
//  listen for network params changes to always have a topical value.

// TrustingPeriod is period through which we can trust a header's validators set.
//
// Should be significantly less than the unbonding period (e.g. unbonding
// period = 3 weeks, trusting period = 2 weeks).
//
// More specifically, trusting period + time needed to check headers + time
// needed to report and punish misbehavior should be less than the unbonding
// period.
var TrustingPeriod = 168 * time.Hour

// IsExpired checks if header is expired against trusting period.
func (h *Header) IsExpired() bool {
	expirationTime := time.Unix(int64(h.Time), 0).Add(TrustingPeriod)
	return !expirationTime.After(time.Now())
}

// VerifyError is thrown on during VerifyAdjacent and VerifyNonAdjacent if verification fails.
type VerifyError struct {
	Reason error
}

func (vr *VerifyError) Error() string {
	return fmt.Sprintf("header: verify: %s", vr.Reason.Error())
}

// VerifyAdjacent validates adjacent untrusted header against trusted header.
func (h *Header) VerifyAdjacent(untrst H) error {
	untrstH, ok := untrst.(*Header)
	if !ok {
		return &VerifyError{
			fmt.Errorf("%T is not of type %T", untrst, h),
		}
	}

	if untrstH.Height != h.Height+1 {
		return &VerifyError{
			fmt.Errorf("headers must be adjacent in height: trusted %d, untrusted %d", h.Height, untrstH.Height),
		}
	}

	if err := verifyNewHeaderAndVals(h, untrstH); err != nil {
		return &VerifyError{Reason: err}
	}

	// Check the validator hashes are the same
	// TODO: next validator set is not available
	if !bytes.Equal(untrstH.AggregatorsHash[:], h.AggregatorsHash[:]) {
		return &VerifyError{
			fmt.Errorf("expected old header next validators (%X) to match those from new header (%X)",
				h.AggregatorsHash,
				untrstH.AggregatorsHash,
			),
		}
	}

	return nil
}

// VerifyNonAdjacent validates non-adjacent untrusted header against trusted header.
func (h *Header) VerifyNonAdjacent(untrst H) error {
	untrstH, ok := untrst.(*Header)
	if !ok {
		return &VerifyError{
			fmt.Errorf("%T is not of type %T", untrst, h),
		}
	}
	if untrstH.Height == h.Height+1 {
		return &VerifyError{
			fmt.Errorf(
				"headers must be non adjacent in height: trusted %d, untrusted %d",
				h.Height,
				untrstH.Height,
			),
		}
	}

	if err := verifyNewHeaderAndVals(h, untrstH); err != nil {
		return &VerifyError{Reason: err}
	}

	// Ensure that untrusted commit has enough of trusted commit's power.
	// err := h.ValidatorSet.VerifyCommitLightTrusting(eh.ChainID, untrst.Commit, light.DefaultTrustLevel)
	// if err != nil {
	// 	return &VerifyError{err}
	// }

	return nil
}

// clockDrift defines how much new header's time can drift into
// the future relative to the now time during verification.
var maxClockDrift = 10 * time.Second

// verifyNewHeaderAndVals performs basic verification of untrusted header.
func verifyNewHeaderAndVals(trusted, untrusted *Header) error {
	if err := untrusted.ValidateBasic(); err != nil {
		return fmt.Errorf("untrusted.ValidateBasic failed: %w", err)
	}

	if untrusted.Height <= trusted.Height {
		return fmt.Errorf("expected new header height %d to be greater than one of old header %d",
			untrusted.Height,
			trusted.Height)
	}

	if !untrusted.Timestamp().After(trusted.Timestamp()) {
		return fmt.Errorf("expected new header time %v to be after old header time %v",
			untrusted.Time,
			trusted.Time)
	}

	if !untrusted.Timestamp().Before(time.Now().Add(maxClockDrift)) {
		return fmt.Errorf("new header has a time from the future %v (now: %v; max clock drift: %v)",
			untrusted.Time,
			time.Now(),
			maxClockDrift)
	}

	return nil
}

// Verify combines both VerifyAdjacent and VerifyNonAdjacent functions.
func (h *Header) Verify(untrst H) error {
	untrstH, ok := untrst.(*Header)
	if !ok {
		return &VerifyError{
			fmt.Errorf("%T is not of type %T", untrst, h),
		}
	}

	if untrstH.Height != h.Height+1 {
		return h.VerifyNonAdjacent(untrst)
	}

	return h.VerifyAdjacent(untrst)
}

// Validate performs basic validation to check for missed/incorrect fields.
func (h *Header) Validate() error {
	return h.ValidateBasic()
}
