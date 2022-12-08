package types

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type A struct {
}

func (a *A) New() H {
	return &A{}
}

func (a *A) Number() int64 {
	return 0
}

func (a *A) Timestamp() time.Time {
	return time.Now()
}

func (a *A) Hash() [32]byte {
	return [32]byte{}
}

func (a *A) IsExpired() bool {
	return false
}

func (a *A) IsRecent(duration time.Duration) bool {
	return true
}

func (a *A) LastHeader() [32]byte {
	return [32]byte{}
}

func (a *A) Verify(h H) error {
	return nil
}

func (a *A) VerifyAdjacent(h H) error {
	return nil
}

func (a *A) VerifyNonAdjacent(h H) error {
	return nil
}

func (a *A) Validate() error {
	return nil
}

func (a *A) MarshalBinary() ([]byte, error) {
	return []byte{}, nil
}

func (a *A) UnmarshalBinary(data []byte) error {
	return nil
}

func TestInterfaceCompatible(t *testing.T) {
	assert := assert.New(t)
	h := &Header{}
	var a H = &A{}
	err := h.VerifyAdjacent(a)
	assert.Error(err)
}

func TestNew(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Time: 123456}

	h2 := h1.New()
	assert.NotNil(h2)
}

func TestHash(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Height: 123456}

	h := [32]byte{104, 232, 90, 84, 228, 141, 242, 116, 213, 125, 19, 72, 23, 49, 5, 255, 5, 82, 174, 209, 213, 171, 106, 106, 156, 227, 119, 225, 24, 130, 129, 185}

	assert.Equal(h, h1.Hash())
}

func TestNumber(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Height: 123456}

	assert.Equal(h1.Number(), int64(123456))
}

func TestLastHeader(t *testing.T) {
	assert := assert.New(t)
	h := [32]byte{104, 232, 90, 84, 228, 141, 242, 116, 213, 125, 19, 72, 23, 49, 5, 255, 5, 82, 174, 209, 213, 171, 106, 106, 156, 227, 119, 225, 24, 130, 129, 185}
	h1 := &Header{LastHeaderHash: h}

	assert.Equal(h1.LastHeader(), h)
}

func TestTimestamp(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Time: 123456}

	t1 := h1.Timestamp()
	assert.Equal(t1.Unix(), int64(123456))
}

func TestIsRecent(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Time: 123456}
	t2 := time.Now()
	h2 := &Header{Time: uint64(t2.Unix())}

	recent := h1.IsRecent(time.Hour)
	assert.False(recent)

	recent = h2.IsRecent(time.Hour)
	assert.True(recent)
}

func TestIsExpired(t *testing.T) {
	assert := assert.New(t)
	h1 := &Header{Time: 123456}
	t2 := time.Now()
	h2 := &Header{Time: uint64(t2.Unix())}

	expired := h1.IsExpired()
	assert.True(expired)

	expired = h2.IsExpired()
	assert.False(expired)
}

var case_adj = []struct {
	name  string
	input *Header
}{
	{"trusted", &Header{ProposerAddress: []byte("123"), Time: 123, Height: 1}},
	{"untrusted", &Header{ProposerAddress: []byte("123"), Time: 456, Height: 2}},
}

var case_non_adj = []struct {
	name  string
	input *Header
}{
	{"trusted", &Header{ProposerAddress: []byte("123"), Time: 123, Height: 1}},
	{"untrusted", &Header{ProposerAddress: []byte("123"), Time: 456, Height: 3}},
}

func TestVerifyAdjacent(t *testing.T) {
	assert := assert.New(t)

	err := case_adj[0].input.VerifyAdjacent(case_adj[1].input)
	assert.NoError(err)

	err = case_non_adj[0].input.VerifyAdjacent(case_non_adj[1].input)
	assert.Error(err)
}

func TestVerifyNonAdjacent(t *testing.T) {
	assert := assert.New(t)

	err := case_non_adj[0].input.VerifyNonAdjacent(case_non_adj[1].input)
	assert.NoError(err)

	err = case_adj[0].input.VerifyNonAdjacent(case_adj[1].input)
	assert.Error(err)
}

func TestVerify(t *testing.T) {
	assert := assert.New(t)

	err := case_adj[0].input.Verify(case_adj[1].input)
	assert.NoError(err)

	err = case_non_adj[0].input.Verify(case_non_adj[1].input)
	assert.NoError(err)
}

func TestValidate(t *testing.T) {
	assert := assert.New(t)
	h := &Header{}
	err := h.Validate()
	assert.Error(err)

	h = &Header{
		ProposerAddress: []byte("123"),
	}
	err = h.Validate()
	assert.NoError(err)
}
