package headertest

import (
	"testing"
	"time"
)

// DummySuite provides everything you need to test chain of DummyHeaders.
// If not, please don't hesitate to extend it for your case.
type DummySuite struct {
	t *testing.T

	head *DummyHeader
}

// NewTestSuite setups a new test suite.
func NewTestSuite(t *testing.T) *DummySuite {
	return &DummySuite{
		t: t,
	}
}

func (s *DummySuite) Head() *DummyHeader {
	if s.head == nil {
		s.head = s.genesis()
	}
	return s.head
}

func (s *DummySuite) GenDummyHeaders(num int) []*DummyHeader {
	headers := make([]*DummyHeader, num)
	for i := range headers {
		headers[i] = s.NextHeader()
	}
	return headers
}

func (s *DummySuite) NextHeader() *DummyHeader {
	if s.head == nil {
		s.head = s.genesis()
		return s.head
	}

	dh := RandDummyHeader(s.t)
	dh.Timestamp = s.head.Time().Add(time.Nanosecond)
	dh.HeightI = s.head.Height() + 1
	dh.PreviousHash = s.head.Hash()
	dh.Chainid = s.head.ChainID()
	_ = dh.rehash()
	s.head = dh
	return s.head
}

func (s *DummySuite) genesis() *DummyHeader {
	return &DummyHeader{
		HeightI:   1,
		Timestamp: time.Now().Add(-10 * time.Second).UTC(),
		Chainid:   "test",
	}
}
