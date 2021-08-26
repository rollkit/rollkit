package registry

import (
	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/da/lazyledger"
	"github.com/lazyledger/optimint/da/mock"
)

// this is a central registry for all Data Availability Layer Clients
var clients = map[string]func() da.DataAvailabilityLayerClient{
	"mock":       func() da.DataAvailabilityLayerClient { return &mock.MockDataAvailabilityLayerClient{} },
	"lazyledger": func() da.DataAvailabilityLayerClient { return &lazyledger.LazyLedger{} },
}

// GetClient returns client identified by name.
func GetClient(name string) da.DataAvailabilityLayerClient {
	return clients[name]()
}
