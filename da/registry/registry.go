package registry

import (
	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/da/lazyledger"
	"github.com/lazyledger/optimint/da/mock"
)

// this is a central registry for all Data Availability Layer Clients
var clients = map[string]da.DataAvailabilityLayerClient{
	"mock":       &mock.MockDataAvailabilityLayerClient{},
	"lazyledger": &lazyledger.LazyLedger{},
}

// GetClient returns client identified by name.
func GetClient(name string) da.DataAvailabilityLayerClient {
	return clients[name]
}
