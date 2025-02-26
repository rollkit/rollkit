package service

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/log"
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

var (
	// ErrAlreadyStarted is returned when somebody tries to start an already running service.
	ErrAlreadyStarted = errors.New("already started")
	// ErrNotStarted is returned when somebody tries to stop a not running service.
	ErrNotStarted = errors.New("not started")
)

// Service defines a service that can be started, stopped, and reset.
type Service interface {
	// Start the service. If it's already started, returns an error.
	Start(context.Context) error
	OnStart(context.Context) error

	// Stop the service. If it's not started, returns an error.
	Stop(context.Context) error
	OnStop(context.Context)

	// Reset the service. Panics by default - must be overwritten to enable reset.
	Reset(context.Context) error
	OnReset(context.Context) error

	// Return true if the service is running.
	IsRunning() bool

	// Quit returns a channel which is closed once the service is stopped.
	Quit() <-chan struct{}

	// String returns a string representation of the service.
	String() string

	// SetLogger sets a logger.
	SetLogger(log.Logger)
}

// BaseService uses a cancellable context to manage service lifetime.
// Instead of manually handling atomic flags and a quit channel,
// we create a context when the service is started and cancel it when stopping.
type BaseService struct {
	Logger log.Logger
	name   string
	impl   Service

	// A cancellable context used to signal the service has stopped.
	ctx    context.Context
	cancel context.CancelFunc
}

// NewBaseService creates a new BaseService.
// The provided implementation (impl) must be the "subclass" that implements OnStart, OnStop, and OnReset.
func NewBaseService(logger log.Logger, name string, impl Service) *BaseService {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	return &BaseService{
		Logger: logger,
		name:   name,
		impl:   impl,
		// ctx and cancel remain nil until Start.
	}
}

// SetLogger sets the logger.
func (bs *BaseService) SetLogger(l log.Logger) {
	bs.Logger = l
}

// Start creates a new cancellable context for the service and calls its OnStart callback.
func (bs *BaseService) Start(ctx context.Context) error {
	if bs.IsRunning() {
		bs.Logger.Debug("service start",
			"msg", fmt.Sprintf("Not starting %v service -- already started", bs.name),
			"impl", bs.impl)
		return ErrAlreadyStarted
	}

	// Create a new cancellable context derived from the provided one.
	bs.ctx, bs.cancel = context.WithCancel(ctx)

	bs.Logger.Info("service start",
		"msg", fmt.Sprintf("Starting %v service", bs.name),
		"impl", bs.impl.String())

	// Call subclass' OnStart to initialize the service.
	if err := bs.impl.OnStart(bs.ctx); err != nil {
		// On error, cancel the context and clear it.
		bs.cancel()
		bs.ctx = nil
		return err
	}
	return nil
}

// OnStart does nothing by default.
func (bs *BaseService) OnStart(ctx context.Context) error { return nil }

// Stop cancels the service's context and calls its OnStop callback.
func (bs *BaseService) Stop(ctx context.Context) error {
	if !bs.IsRunning() {
		bs.Logger.Error(fmt.Sprintf("Not stopping %v service -- has not been started yet", bs.name),
			"impl", bs.impl)
		return ErrNotStarted
	}

	bs.Logger.Info("service stop",
		"msg", fmt.Sprintf("Stopping %v service", bs.name),
		"impl", bs.impl)

	// Call subclass' OnStop method.
	bs.impl.OnStop(bs.ctx)

	// Cancel the context to signal all routines waiting on ctx.Done() that the service is stopping.
	bs.cancel()
	bs.ctx = nil

	return nil
}

// OnStop does nothing by default.
func (bs *BaseService) OnStop(ctx context.Context) {}

// Reset recreates the service's context after verifying that it is not running.
func (bs *BaseService) Reset(ctx context.Context) error {
	if bs.IsRunning() {
		bs.Logger.Debug("service reset",
			"msg", fmt.Sprintf("Can't reset %v service. Service is running", bs.name),
			"impl", bs.impl)
		return fmt.Errorf("cannot reset running service %s", bs.name)
	}

	// Call the subclass' OnReset callback.
	return bs.impl.OnReset(ctx)
}

// OnReset panics by default.
func (bs *BaseService) OnReset(ctx context.Context) error {
	panic("The service cannot be reset without implementing OnReset")
}

// IsRunning returns true when the service's context is not nil.
func (bs *BaseService) IsRunning() bool {
	return bs.ctx != nil
}

// Quit returns the done channel of the service's context.
// When the context is cancelled during Stop, the channel is closed.
func (bs *BaseService) Quit() <-chan struct{} {
	if bs.ctx != nil {
		return bs.ctx.Done()
	}
	// In case the service is not running, return a closed channel.
	ch := make(chan struct{})
	close(ch)
	return ch
}

// String returns the service name.
func (bs *BaseService) String() string {
	return bs.name
}
