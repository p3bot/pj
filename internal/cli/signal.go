package cli

import (
	"context"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// caughtSignal: interrupt is intent (POSIX 128+signum), not a retryable failure.
var caughtSignal atomic.Int32

func signalContext() (context.Context, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	go func() {
		select {
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if s, ok := sig.(syscall.Signal); ok {
				caughtSignal.Store(int32(s))
			}
			cancel()
		case <-ctx.Done():
			// stop() cancelled; exit rather than park on ch.
		}
	}()
	return ctx, func() {
		signal.Stop(ch)
		cancel()
	}
}

// SignalExitCode returns 128+signum when interrupted, else 0 (checked before error map).
func SignalExitCode() int {
	if s := caughtSignal.Load(); s != 0 {
		return 128 + int(s)
	}
	return 0
}
