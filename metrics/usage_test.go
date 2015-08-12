package metrics_test

import (
	"context"
	"io/ioutil"
	"log"
	"time"

	"github.com/Netflix/metrics-client-go/metrics"
)

var (
	global = map[string]string{"global": "tag"}
	tags   = map[string]string{"foo": "bar"}
)

func Example() {
	ctx, cancel := context.WithCancel(context.Background())
	// see WithURLInterval for customizations
	m := metrics.New(ctx, log.New(ioutil.Discard, "metrics: ", log.LstdFlags), global)

	// tags can also be nil
	m.Gauge("gauge.name", 15, tags)
	m.Counter("counter.name", 1, tags)
	m.Timer("foo.elapsed", 5*time.Minute, tags)

	// ...

	// later:
	m.Flush()
	cancel() // stops all metrics processing, all operations become a noop
}
