package registry

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegistery(t *testing.T) {
	assert := assert.New(t)

	expected := []string{"mock", "grpc", "celestia"}
	actual := RegisteredClients()

	assert.ElementsMatch(expected, actual)

	for _, e := range expected {
		dalc := GetClient(e)
		assert.NotNil(dalc)
	}

	assert.Nil(GetClient("nonexistent"))
}
