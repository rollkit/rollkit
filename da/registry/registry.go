package registry

import (
	"github.com/lazyledger/optimint/da"
	"github.com/lazyledger/optimint/da/lazyledger"
	"github.com/lazyledger/optimint/da/mock"
)

var clients = map[string]da.DataAvailabilityLayerClient{
	"mock":       &mock.MockDataAvailabilityLayerClient{},
	"lazyledger": &lazyledger.LazyLedger{},
}

func GetClient(name string) da.DataAvailabilityLayerClient {
	return clients[name]
}
