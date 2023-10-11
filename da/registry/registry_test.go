package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/mock"
)

func TestRegistery(t *testing.T) {
	assert := assert.New(t)

	expected := []string{"mock", "grpc", "celestia", "newda"}
	actual := RegisteredClients()

	assert.ElementsMatch(expected, actual)

	constructor := func() da.DataAvailabilityLayerClient {
		return &mock.DataAvailabilityLayerClient{} // cheating, only for tests :D
	}
	err := Register("testDA", constructor)
	assert.NoError(err)

	// re-registration should fail
	err = Register("celestia", constructor)
	regErr := &ErrAlreadyRegistered{}
	assert.ErrorAs(err, &regErr)
	assert.Equal("celestia", regErr.name)

	assert.Contains(RegisteredClients(), "testDA")

	for _, e := range RegisteredClients() {
		dalc := GetClient(e)
		assert.NotNil(dalc)
	}

	assert.Nil(GetClient("nonexistent"))
}
