package service

import (
	"context"
	"fmt"

	logging "github.com/ipfs/go-log/v2"
)

/*
type FooService struct {
	BaseService
	// extra fields for FooService
}
func NewFooService(logger log.Logger) *FooService {
	fs := &FooService{}
	fs.BaseService = *NewBaseService(logger, "FooService", fs)
	return fs
}
func (fs *FooService) OnStart(ctx context.Context) error {
	// initialize fields, start goroutines, etc.
	go func() {
		<-ctx.Done()
		// cleanup when context is cancelled
	}()
	return nil
}
func (fs *FooService) OnStop(ctx context.Context) {
	// stop routines, cleanup resources, etc.
}
func (fs *FooService) OnReset(ctx context.Context) error {
	// implement reset if desired
	return nil
}
*/

// Service exposes a Run method that blocks until the service ends or the context is canceled.
type Service interface {
	// Run starts the service and blocks until it is shut down via context cancellation,
	// an error occurs, or all work is done.
	Run(ctx context.Context) error
}

// BaseService provides a basic implementation of the Service interface.
type BaseService struct {
	Logger logging.EventLogger
	name   string
	impl   Service // Implementation that can override Run behavior
}

// NewBaseService creates a new BaseService.
// The provided implementation (impl) should be the "subclass" that implements Run.
func NewBaseService(logger logging.EventLogger, name string, impl Service) *BaseService {
	if logger == nil {
		// For ipfs/go-log, a "Nop" equivalent would be a logger with level set to FATAL,
		// or a logger with a nil core. For simplicity, we'll create a default logger
		// and set its level if no logger is provided.
		nopLogger := logging.Logger("nop")
		_ = logging.SetLogLevel("nop", "FATAL") // Attempt NOP behavior
		logger = nopLogger
	}
	return &BaseService{
		Logger: logger,
		name:   name,
		impl:   impl,
	}
}

// SetLogger sets the logger.
func (bs *BaseService) SetLogger(l logging.EventLogger) {
	bs.Logger = l
}

// Run implements the Service interface. It logs the start of the service,
// then defers to the implementation's Run method to do the actual work.
// If impl is nil or the same as bs, it uses the default implementation.
func (bs *BaseService) Run(ctx context.Context) error {
	bs.Logger.Info("service start",
		"msg", fmt.Sprintf("Starting %v service", bs.name),
		"impl", bs.name)

	// If the implementation is nil or is the BaseService itself,
	// use the default implementation which just waits for context cancellation
	if bs.impl == nil || bs.impl == bs {
		<-ctx.Done()
		bs.Logger.Info("service stop",
			"msg", fmt.Sprintf("Stopping %v service", bs.name),
			"impl", bs.name)
		return ctx.Err()
	}

	// Otherwise, call the implementation's Run method
	return bs.impl.Run(ctx)
}

// String returns the service name.
func (bs *BaseService) String() string {
	return bs.name
}
