package registry

import (
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/da/celestia"
	"github.com/celestiaorg/optimint/da/mock"
)

// this is a central registry for all Data Availability Layer Clients
var clients = map[string]func() da.DataAvailabilityLayerClient{
	"mock":     func() da.DataAvailabilityLayerClient { return &mock.MockDataAvailabilityLayerClient{} },
	"celestia": func() da.DataAvailabilityLayerClient { return &celestia.Celestia{} },
}

// GetClient returns client identified by name.
func GetClient(name string) da.DataAvailabilityLayerClient {
	return clients[name]()
}
