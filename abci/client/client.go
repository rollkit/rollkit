package client

import (
	"context"

	"github.com/lazyledger/lazyledger-core/abci/types"
)

type Client interface {
	Echo(ctx context.Context, msg string) (*types.ResponseEcho, error)
}
