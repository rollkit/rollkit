//go:build !integration

package node

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	logging "github.com/ipfs/go-log/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/pkg/service"
)

func TestStartInstrumentationServer(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	assert := assert.New(t)

	var config = getTestConfig(t, 1)
	config.Instrumentation.Prometheus = true
	config.Instrumentation.PrometheusListenAddr = "127.0.0.1:26660"
	config.Instrumentation.Pprof = true
	config.Instrumentation.PprofListenAddr = "127.0.0.1:26661"

	node := &FullNode{
		nodeConfig:  config,
		BaseService: *service.NewBaseService(logging.Logger("test"), "TestNode", nil), // Use ipfs/go-log
	}
	// Set log level for testing if needed, e.g., logging.SetLogLevel("test", "debug")

	prometheusSrv, pprofSrv := node.startInstrumentationServer()

	require.NotNil(prometheusSrv, "Prometheus server should be initialized")
	require.NotNil(pprofSrv, "Pprof server should be initialized")

	time.Sleep(100 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://%s/metrics", config.Instrumentation.PrometheusListenAddr))
	require.NoError(err, "Failed to get Prometheus metrics")
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			t.Logf("Error closing response body: %v", err)
		}
	}()
	assert.Equal(http.StatusOK, resp.StatusCode, "Prometheus metrics endpoint should return 200 OK")
	body, err := io.ReadAll(resp.Body)
	require.NoError(err)
	assert.Contains(string(body), "# HELP", "Prometheus metrics body should contain HELP lines") // Check for typical metrics content

	resp, err = http.Get(fmt.Sprintf("http://%s/debug/pprof/", config.Instrumentation.PprofListenAddr))
	require.NoError(err, "Failed to get Pprof index")
	defer func() {
		err := resp.Body.Close()
		if err != nil {
			t.Logf("Error closing response body: %v", err)
		}
	}()
	assert.Equal(http.StatusOK, resp.StatusCode, "Pprof index endpoint should return 200 OK")
	body, err = io.ReadAll(resp.Body)
	require.NoError(err)
	assert.Contains(string(body), "Types of profiles available", "Pprof index body should contain expected text")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	if prometheusSrv != nil {
		err = prometheusSrv.Shutdown(shutdownCtx)
		assert.NoError(err, "Prometheus server shutdown should not return error")
	}
	if pprofSrv != nil {
		err = pprofSrv.Shutdown(shutdownCtx)
		assert.NoError(err, "Pprof server shutdown should not return error")
	}
}
