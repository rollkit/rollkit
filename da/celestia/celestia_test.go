package celestia

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rollkit/rollkit/da"
)

func TestDataRequestErrorToStatus(t *testing.T) {
	assert := assert.New(t)
	assert.Equal(da.StatusSuccess, dataRequestErrorToStatus(da.ErrNamespaceNotFound))
	assert.Equal(da.StatusNotFound, dataRequestErrorToStatus(da.ErrDataNotFound))
	assert.Equal(da.StatusNotFound, dataRequestErrorToStatus(da.ErrEDSNotFound))
	assert.Equal(da.StatusError, dataRequestErrorToStatus(errors.New("some random error")))
}
