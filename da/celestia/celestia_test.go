package celestia

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rollkit/rollkit/da"
)

func TestDataRequestErrorToStatus(t *testing.T) {
	randErr := errors.New("some random error")
	var test = []struct {
		statusCode da.StatusCode
		err        error
	}{
		// Status Success Cases
		{da.StatusSuccess, nil},
		{da.StatusSuccess, da.ErrNamespaceNotFound},
		{da.StatusSuccess, errors.Join(randErr, da.ErrNamespaceNotFound, randErr)},

		// TODO: cases that need investigating, are these possible? If
		// so, is this the correct status code?
		{da.StatusSuccess, errors.Join(da.ErrEDSNotFound, da.ErrNamespaceNotFound)},
		{da.StatusSuccess, errors.Join(da.ErrDataNotFound, da.ErrNamespaceNotFound)},

		// Status not Found Cases
		{da.StatusNotFound, da.ErrDataNotFound},
		{da.StatusNotFound, da.ErrEDSNotFound},
		{da.StatusNotFound, errors.Join(da.ErrEDSNotFound, da.ErrDataNotFound)},
		{da.StatusNotFound, errors.Join(da.ErrEDSNotFound, randErr)},
		{da.StatusNotFound, errors.Join(randErr, da.ErrDataNotFound)},

		// Status Error Cases
		{da.StatusError, randErr},
	}
	for _, tt := range test {
		t.Logf("Testing %v", tt.err)
		assert.Equal(t, tt.statusCode, dataRequestErrorToStatus(tt.err))
	}
}
