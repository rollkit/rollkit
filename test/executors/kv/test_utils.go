package executor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"cosmossdk.io/log"
	coreexecutor "github.com/rollkit/rollkit/core/execution"
	"github.com/spf13/viper"
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

	// Start HTTP server for transaction submission if address is specified
	httpAddr := viper.GetString("kv-executor-http")
	if httpAddr != "" {
		httpServer := NewHTTPServer(kvExecutor, httpAddr)
		logger.Info("Creating KV Executor HTTP server", "address", httpAddr)
		go func() {
			logger.Info("Starting KV Executor HTTP server", "address", httpAddr)
			if err := httpServer.Start(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
				logger.Error("KV Executor HTTP server error", "error", err)
			}
			logger.Info("KV Executor HTTP server stopped", "address", httpAddr)
		}()
	}

	return kvExecutor
}
