package registry

import (
	"fmt"

	"github.com/rollkit/rollkit/da"
	"github.com/rollkit/rollkit/da/avail"
	"github.com/rollkit/rollkit/da/celestia"

	"github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/mock"
)

// ErrAlreadyRegistered is used when user tries to register DA using a name already used in registry.
type ErrAlreadyRegistered struct {
	name string
}

func (e *ErrAlreadyRegistered) Error() string {
	return fmt.Sprintf("Data Availability Layer '%s' already registered", e.name)
}

// this is a central registry for all Data Availability Layer Clients
var clients = map[string]func() da.DataAvailabilityLayerClient{
	"mock":     func() da.DataAvailabilityLayerClient { return &mock.DataAvailabilityLayerClient{} },
	"grpc":     func() da.DataAvailabilityLayerClient { return &grpc.DataAvailabilityLayerClient{} },
	"celestia": func() da.DataAvailabilityLayerClient { return &celestia.DataAvailabilityLayerClient{} },
	"avail":    func() da.DataAvailabilityLayerClient { return &avail.DataAvailabilityLayerClient{} },
}

// GetClient returns client identified by name.
func GetClient(name string) da.DataAvailabilityLayerClient {
	f, ok := clients[name]
	if !ok {
		return nil
	}
	return f()
}

// Register adds a Data Availability Layer Client to registry.
//
// If name was previously used in the registry, error is returned.
func Register(name string, constructor func() da.DataAvailabilityLayerClient) error {
	if _, found := clients[name]; !found {
		clients[name] = constructor
		return nil
	}
	return &ErrAlreadyRegistered{name: name}
}

// RegisteredClients returns names of all DA clients in registry.
func RegisteredClients() []string {
	registered := make([]string, 0, len(clients))
	for name := range clients {
		registered = append(registered, name)
	}
	return registered
}
