package context

import (
	"context"
	"fmt"
	"runtime"
	"strings"
)

// WithDebugCancelFunc returns a custom cancel function with debugging information.
func WithDebugCancelFunc(ctx context.Context) (context.Context, context.CancelFunc) {
	childCtx, cancelFunc := context.WithCancel(ctx)
	debugCancelFunc := func() {
		var pcs [5]uintptr // Capture the last 5 frames
		n := runtime.Callers(1, pcs[:])
		frames := runtime.CallersFrames(pcs[:n])

		var trace []string
		for {
			frame, more := frames.Next()
			trace = append(trace, fmt.Sprintf("%s:%d %s", frame.File, frame.Line, frame.Function))
			if !more {
				break
			}
		}

		fmt.Printf("Context canceled by the following last 5 levels of the stack trace:\n%s\n", strings.Join(trace, "\n"))
		cancelFunc() // Call the standard context cancel function
	}
	return childCtx, debugCancelFunc
}
