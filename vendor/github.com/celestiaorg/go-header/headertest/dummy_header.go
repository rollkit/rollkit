package headertest

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"golang.org/x/crypto/sha3"

	"github.com/celestiaorg/go-header"
)

var ErrDummyVerify = errors.New("dummy verify error")

type DummyHeader struct {
	Chainid      string
	PreviousHash header.Hash
	HeightI      uint64
	Timestamp    time.Time

	hash header.Hash

	// VerifyFailure allows for testing scenarios where a header would fail
	// verification. When set to true, it forces a failure.
	VerifyFailure bool
	// SoftFailure allows for testing scenarios where a header would fail
	// verification with SoftFailure set to true
	SoftFailure bool
}

func RandDummyHeader(t *testing.T) *DummyHeader {
	t.Helper()

	dh := &DummyHeader{
		PreviousHash: RandBytes(32),
		HeightI:      randUint63(),
		Timestamp:    time.Now().UTC(),
	}
	err := dh.rehash()
	if err != nil {
		t.Fatal(err)
	}
	return dh
}

func (d *DummyHeader) New() *DummyHeader {
	return new(DummyHeader)
}

func (d *DummyHeader) IsZero() bool {
	return d == nil
}

func (d *DummyHeader) ChainID() string {
	return d.Chainid
}

func (d *DummyHeader) Hash() header.Hash {
	if len(d.hash) == 0 {
		if err := d.rehash(); err != nil {
			panic(err)
		}
	}
	return d.hash
}

func (d *DummyHeader) rehash() error {
	b, err := d.MarshalBinary()
	if err != nil {
		return err
	}
	hash := sha3.Sum512(b)
	d.hash = hash[:]
	return nil
}

func (d *DummyHeader) Height() uint64 {
	return d.HeightI
}

func (d *DummyHeader) LastHeader() header.Hash {
	return d.PreviousHash
}

func (d *DummyHeader) Time() time.Time {
	return d.Timestamp
}

func (d *DummyHeader) IsRecent(blockTime time.Duration) bool {
	return time.Since(d.Time()) <= blockTime
}

func (d *DummyHeader) IsExpired(period time.Duration) bool {
	expirationTime := d.Time().Add(period)
	return expirationTime.Before(time.Now())
}

func (d *DummyHeader) Verify(hdr *DummyHeader) error {
	if hdr.VerifyFailure {
		return &header.VerifyError{Reason: ErrDummyVerify, SoftFailure: hdr.SoftFailure}
	}
	return nil
}

func (d *DummyHeader) Validate() error {
	return nil
}

func (d *DummyHeader) MarshalBinary() ([]byte, error) {
	return json.Marshal(d)
}

func (d *DummyHeader) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, d)
}

// RandBytes returns slice of n-bytes, or nil in case of error
func RandBytes(n int) []byte {
	buf := make([]byte, n)

	c, err := rand.Read(buf)
	if err != nil || c != n {
		return nil
	}

	return buf
}

func randUint63() uint64 {
	var buf [8]byte

	_, err := rand.Read(buf[:])
	if err != nil {
		return math.MaxInt64
	}

	return binary.BigEndian.Uint64(buf[:]) & math.MaxInt64
}
