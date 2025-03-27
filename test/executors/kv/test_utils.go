package executor

import (
	"context"
	"fmt"
	"os"

	"cosmossdk.io/log"

	coreexecutor "github.com/rollkit/rollkit/core/execution"
)

var logger = log.NewLogger(os.Stdout)

// CreateDirectKVExecutor creates a KVExecutor for testing
func CreateDirectKVExecutor(ctx context.Context) coreexecutor.Executor {
	kvExecutor := NewKVExecutor()

	// Pre-populate with some test transactions
	for i := 0; i < 5; i++ {
		tx := []byte(fmt.Sprintf("test%d=value%d", i, i))
		kvExecutor.InjectTx(tx)
	}

	return kvExecutor
}
