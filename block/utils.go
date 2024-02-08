package block

import (
	"testing"

	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/da"
	test "github.com/rollkit/rollkit/test/log"
)

// Returns a minimalistic block manager
func getManager(t *testing.T) *Manager {
	logger := test.NewFileLoggerCustom(t, test.TempLogFileName(t, t.Name()))
	return &Manager{
		dalc:       &da.DAClient{DA: goDATest.NewDummyDA(), GasPrice: -1, Logger: logger},
		blockCache: NewBlockCache(),
		logger:     logger,
	}
}
