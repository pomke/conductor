package conductor

import (
	"os"
	"os/signal"
	"syscall"
	"time"
)

// Sets the time allowed for a service to start before timing out
func StartupTimeout(d time.Duration) func(*conductor) {
	return func(c *conductor) {
		c.startTimeout = d
	}
}

// Sets the time allowed for a service to stop before timing out
func ShutdownTimeout(d time.Duration) func(*conductor) {
	return func(c *conductor) {
		c.stopTimeout = d
	}
}

// tells the conductor to log output
func Noisy() func(*conductor) {
	return func(c *conductor) {
		c.noisy = true
	}
}

// This hooks SIGTERM and SIGINT and will shut down the conductor
// if one is detected.
func HookSignals() func(*conductor) {
	return func(c *conductor) {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
		go func() {
			select {
			case sig := <-sigCh: // sigterm/sigint caught
				c.logf("Caught %v signal, shutting down", sig)
				c.Stop()
				return
			case <-c.shutdown: // service is closing down..
				return
			}
		}()
	}
}
