package lcn

import (
	"github.com/LastL2/cuberollkit/lcn/types"
	"github.com/LastL2/cuberollkit/third_party/log"
)

// LcnClient is a new settlement implementation;
// Modeled after DAClient in pkg da
type LcnClient struct {
	// TO-DO: Add LcnClient fields
	Config *Config
	Logger log.Logger
}

// Ensure LcnClient implements the Settler interface
var _ Settler = &LcnClient{}

func NewLcnClient(config *Config, logger log.Logger) *LcnClient {
	return &LcnClient{
		Config: config,
		Logger: logger,
	}
}

// TO-DO
func (c *LcnClient) OnStart() error {
	return nil
}

// TO-DO
func (c *LcnClient) OnStop() error {
	return nil
}

// TO-DO
func (c *LcnClient) SubmitBatch() types.ResultSubmitBatch {
	return types.ResultSubmitBatch{}
}

// TO-DO
func (c *LcnClient) RetrieveBatch() types.ResultRetrieveBatch {
	return types.ResultRetrieveBatch{}
}
