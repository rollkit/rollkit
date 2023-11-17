package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/newda"
)

func TestRegistry(t *testing.T) {
	expected := []string{"grpc", "celestia", "newda"}

	actual := RegisteredClients()

	assert.ElementsMatch(t, expected, actual)

	constructor := func() da.DataAvailabilityLayerClient {
		return &newda.NewDA{DA: test.NewDummyDA()} // cheating, only for tests :D
	}
	err := Register("testDA", constructor)
	assert.NoError(t, err)

	// re-registration should fail
	err = Register("celestia", constructor)
	regErr := &ErrAlreadyRegistered{}
	assert.ErrorAs(t, err, &regErr)
	assert.Equal(t, "celestia", regErr.name)

	assert.Contains(t, RegisteredClients(), "testDA")

	for _, e := range RegisteredClients() {
		dalc := GetClient(e)
		assert.NotNil(t, dalc)
	}

	assert.Nil(t, GetClient("nonexistent"))
}
