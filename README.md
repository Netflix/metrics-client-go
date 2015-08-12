A go client for the Atlasd sidecar API on localhost. This package requires that you install Atlasd on the given system, and that Atlasd is running on the expected port.

# Metrics

Usage:

```go
ctx, cancel := context.WithCancel(context.Background())
// see WithURLInterval for customizations
m := metrics.New(ctx, log.New(ioutil.Discard, "metrics: ", log.LstdFlags))

m.Gauge("gauge.name", tags, 15)
m.Counter("counter.name", tags, 1)
m.Timer("foo.elapsed", tags, 5*time.Minute)

// ...

// later:
m.Flush()
cancel() // stops all metrics processing, all operations become a noop
```

