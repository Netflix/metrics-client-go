package global

import (
	"context"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/Netflix/metrics-client-go/metrics"
)

var (
	mu       sync.Mutex
	reporter metrics.Reporter
	cancel   context.CancelFunc
)

func init() {
	var ctx context.Context
	ctx, cancel = context.WithCancel(context.Background()) // nolint: vet
	reporter = metrics.New(ctx, log.WithField("name", "atlas-client"), nil)
} // nolint: vet

// Disable replaces the global metrics.Reporter with a noop
//
// Deprecated: use non-global metrics.Reporter instances
func Disable() {
	replace(metrics.Discard, func() {})
}

// SetLogger replaces the global metrics.Reporter with one using a custom metrics.Logger
//
// Deprecated: use non-global metrics.Reporter instances
func SetLogger(l metrics.Logger) {
	ctx, newCancel := context.WithCancel(context.Background())
	reporter = metrics.New(ctx, l, nil)
	replace(reporter, newCancel)
}

// SetLoggerURL replaces the global metrics.Reporter with one using a custom metrics.Logger and URL
//
// Deprecated: use non-global metrics.Reporter instances
func SetLoggerURL(l metrics.Logger, url string) {
	ctx, newCancel := context.WithCancel(context.Background())
	reporter = metrics.WithURL(ctx, l, url, nil)
	replace(reporter, newCancel)
}

func replace(new metrics.Reporter, newCancel context.CancelFunc) {
	mu.Lock()
	defer mu.Unlock()

	reporter.Flush()
	cancel()

	reporter = new
	cancel = newCancel
}

// Counter is a global wrapper of the metrics package.
//
// Deprecated: use non-global metrics.Reporter instances
func Counter(name string, value int, tags map[string]string) {
	reporter.Counter(name, value, tags)
}

// Gauge is a global wrapper of the metrics package.
//
// Deprecated: use non-global metrics.Reporter instances
func Gauge(name string, value int, tags map[string]string) {
	reporter.Gauge(name, value, tags)
}

// Timer is a global wrapper of the metrics package.
//
// Deprecated: use non-global metrics.Reporter instances
func Timer(name string, value time.Duration, tags map[string]string) {
	reporter.Timer(name, value, tags)
}

// Flush is a global wrapper of the metrics package.
//
// Deprecated: use non-global metrics.Reporter instances
func Flush() {
	reporter.Flush()
}
