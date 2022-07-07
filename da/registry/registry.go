package registry

import (
	"github.com/celestiaorg/optimint/da"
	"github.com/celestiaorg/optimint/da/celestia"
	"github.com/celestiaorg/optimint/da/grpc"
	"github.com/celestiaorg/optimint/da/mock"
)

// this is a central registry for all Data Availability Layer Clients
var clients = map[string]func() da.DataAvailabilityLayerClient{
	"mock":     func() da.DataAvailabilityLayerClient { return &mock.DataAvailabilityLayerClient{} },
	"grpc":     func() da.DataAvailabilityLayerClient { return &grpc.DataAvailabilityLayerClient{} },
	"celestia": func() da.DataAvailabilityLayerClient { return &celestia.DataAvailabilityLayerClient{} },
}

// GetClient returns client identified by name.
func GetClient(name string) da.DataAvailabilityLayerClient {
	f, ok := clients[name]
	if !ok {
		return nil
	}
	return f()
}

// RegisteredClients returns names of all DA clients in registry.
func RegisteredClients() []string {
	registered := make([]string, 0, len(clients))
	for name := range clients {
		registered = append(registered, name)
	}
	return registered
}
