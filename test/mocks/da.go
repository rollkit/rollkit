package mocks

import (
	"github.com/cometbft/cometbft/libs/log"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
)

// MockServerAddr is the address of the mock server.
var MockServerAddr = ":7980"

// GetMockDA returns a new DA client with a dummy DA.
func GetMockDA() *da.DAClient {
	return &da.DAClient{DA: goDATest.NewDummyDA(), Logger: log.TestingLogger()}
}
