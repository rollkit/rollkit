package log

import (
	"io"
	"os"

	ipfslog "github.com/ipfs/go-log/v2"
	"github.com/rs/zerolog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger defines the logging interface compatible with github.com/rollkit/rollkit/pkg/log
type Logger interface {
	// Info takes a message and a set of key/value pairs and logs with level INFO.
	// The key of the tuple must be a string.
	Info(msg string, keyVals ...any)

	// Warn takes a message and a set of key/value pairs and logs with level WARN.
	// The key of the tuple must be a string.
	Warn(msg string, keyVals ...any)

	// Error takes a message and a set of key/value pairs and logs with level ERR.
	// The key of the tuple must be a string.
	Error(msg string, keyVals ...any)

	// Debug takes a message and a set of key/value pairs and logs with level DEBUG.
	// The key of the tuple must be a string.
	Debug(msg string, keyVals ...any)

	// With returns a new wrapped logger with additional context provided by a set.
	With(keyVals ...any) Logger

	// Impl returns the underlying logger implementation.
	// It is used to access the full functionalities of the underlying logger.
	// Advanced users can type cast the returned value to the actual logger.
	Impl() any
}

// zapLogger wraps ipfs/go-log ZapEventLogger to implement our Logger interface
type zapLogger struct {
	logger *ipfslog.ZapEventLogger
}

// NewLogger creates a new logger that writes to the given destination.
// This function maintains compatibility with github.com/rollkit/rollkit/pkg/log.NewLogger().
func NewLogger(dst io.Writer, options ...Option) Logger {
	// Apply configuration options
	config := &Config{
		Level:      zapcore.Level(zapcore.InfoLevel),
		EnableJSON: false,
		Trace:      false,
	}
	for _, opt := range options {
		opt(config)
	}
	
	// If dst is provided and is not os.Stdout, we need a custom logger that writes to dst
	if dst != nil && dst != os.Stdout {
		// Create a custom zap logger that writes to the destination
		encoderConfig := zap.NewProductionEncoderConfig()
		encoderConfig.TimeKey = "timestamp"
		encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
		
		var encoder zapcore.Encoder
		if config.EnableJSON {
			encoder = zapcore.NewJSONEncoder(encoderConfig)
		} else {
			encoder = zapcore.NewConsoleEncoder(encoderConfig)
		}
		
		core := zapcore.NewCore(encoder, zapcore.AddSync(dst), config.Level)
		zapLog := zap.New(core)
		sugaredLogger := zapLog.Sugar()
		
		return &zapLogger{
			logger: &ipfslog.ZapEventLogger{SugaredLogger: *sugaredLogger},
		}
	}
	
	// For compatibility, if dst is os.Stdout or nil, use ipfs/go-log's default logger
	logger := ipfslog.Logger("rollkit")
	
	// Set log level if specified in config
	switch config.Level {
	case zapcore.DebugLevel:
		ipfslog.SetLogLevel("rollkit", "debug")
	case zapcore.InfoLevel:
		ipfslog.SetLogLevel("rollkit", "info")
	case zapcore.WarnLevel:
		ipfslog.SetLogLevel("rollkit", "warn")
	case zapcore.ErrorLevel:
		ipfslog.SetLogLevel("rollkit", "error")
	}
	
	return &zapLogger{
		logger: logger,
	}
}

// NewNopLogger creates a no-op logger.
func NewNopLogger() Logger {
	// Create a zap logger that discards all output
	zapCore := zapcore.NewNopCore()
	zapLog := zap.New(zapCore).Sugar()
	return &zapLogger{
		logger: &ipfslog.ZapEventLogger{SugaredLogger: *zapLog},
	}
}

// NewTestLogger creates a test logger.
func NewTestLogger(t TestingT) Logger {
	// Use ipfs/go-log's default logger for tests
	return &zapLogger{
		logger: ipfslog.Logger("test"),
	}
}

// NewTestLoggerInfo creates a test logger at info level.
func NewTestLoggerInfo(t TestingT) Logger {
	return NewTestLogger(t)
}

// NewTestLoggerError creates a test logger at error level.
func NewTestLoggerError(t TestingT) Logger {
	return NewTestLogger(t)
}

// Info logs at info level with key-value pairs
func (z *zapLogger) Info(msg string, keyVals ...any) {
	z.logger.Infow(msg, keyVals...)
}

// Warn logs at warn level with key-value pairs
func (z *zapLogger) Warn(msg string, keyVals ...any) {
	z.logger.Warnw(msg, keyVals...)
}

// Error logs at error level with key-value pairs
func (z *zapLogger) Error(msg string, keyVals ...any) {
	z.logger.Errorw(msg, keyVals...)
}

// Debug logs at debug level with key-value pairs
func (z *zapLogger) Debug(msg string, keyVals ...any) {
	z.logger.Debugw(msg, keyVals...)
}

// With returns a new logger with additional context
func (z *zapLogger) With(keyVals ...any) Logger {
	sugaredLogger := z.logger.With(keyVals...)
	return &zapLogger{
		logger: &ipfslog.ZapEventLogger{SugaredLogger: *sugaredLogger},
	}
}

// Impl returns the underlying logger implementation
func (z *zapLogger) Impl() any {
	return z.logger
}

// Option defines configuration options for the logger
type Option func(*Config)

// Config holds logger configuration
type Config struct {
	Level      zapcore.Level
	Format     string
	EnableJSON bool
	Trace      bool
}

// OutputJSONOption enables JSON output format
func OutputJSONOption() Option {
	return func(c *Config) {
		c.EnableJSON = true
	}
}

// LevelOption sets the log level
func LevelOption(level zerolog.Level) Option {
	return func(c *Config) {
		// Convert zerolog level to zapcore level
		switch level {
		case zerolog.DebugLevel:
			c.Level = zapcore.DebugLevel
		case zerolog.InfoLevel:
			c.Level = zapcore.InfoLevel
		case zerolog.WarnLevel:
			c.Level = zapcore.WarnLevel
		case zerolog.ErrorLevel:
			c.Level = zapcore.ErrorLevel
		default:
			c.Level = zapcore.InfoLevel
		}
	}
}

// TraceOption enables or disables stack traces
func TraceOption(enabled bool) Option {
	return func(c *Config) {
		c.Trace = enabled
	}
}

// ColorOption enables or disables colored output
func ColorOption(enabled bool) Option {
	return func(c *Config) {
		// For compatibility - ipfs/go-log handles coloring automatically
		// This option is kept for API compatibility but doesn't affect output
	}
}

// TestingT is an interface for testing.T
type TestingT interface {
	Log(args ...any)
	Logf(format string, args ...any)
	Error(args ...any)
	Errorf(format string, args ...any)
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}