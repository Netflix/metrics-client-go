package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type Metric struct {
	Name      string
	Value     int
	Type      string
	Timestamp int64
	Tags      map[string]string
}

func recordMetricsReceived(responseBody []byte, numCounters, numGauges, numTimers *uint64) {
	metrics := []*Metric{}
	err := json.Unmarshal(responseBody, &metrics)
	if err != nil {
		fmt.Printf("Error in Unmarshalling response body %s\n", string(responseBody))
	}
	for _, m := range metrics {
		if m.Type == "COUNTER" {
			atomic.AddUint64(numCounters, 1)
		} else if m.Type == "GAUGE" {
			atomic.AddUint64(numGauges, 1)
		} else if m.Type == "TIMER" {
			atomic.AddUint64(numTimers, 1)
		}
	}
}

func makeMockHandler(numCounters, numGauges, numTimers *uint64) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path[1:] != "metrics" {
			http.NotFound(w, r)
			return
		}
		b, err := ioutil.ReadAll(r.Body)
		if err == nil {
			recordMetricsReceived(b, numCounters, numGauges, numTimers)
		}
	}
}

func pushTestMetrics(r Reporter) {
	counters := []*counter{
		{Timestamp: 0, Name: "testcounter", Value: 2, Type: "COUNTER", Tags: map[string]string{"appname": "testapp"}},
		{Timestamp: 2, Name: "testcounter", Value: 5, Type: "COUNTER", Tags: map[string]string{"appname": "testapp"}},
	}
	gauges := []*gauge{
		{Timestamp: 0, Name: "testGauge", Value: 100, Type: "GAUGE", Tags: map[string]string{"appname": "testapp"}},
		{Timestamp: 0, Name: "testGauge", Value: 100, Type: "GAUGE", Tags: map[string]string{"appname": "testapp"}},
	}
	timers := []*timer{
		{Timestamp: 0, Name: "testTimer", Value: 123454, Type: "TIMER", Tags: map[string]string{"appname": "testapp"}},
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for _, c := range counters {
			r.Counter(c.Name, int(c.Value), c.Tags)
		}
	}()
	go func() {
		defer wg.Done()
		for _, g := range gauges {
			r.Gauge(g.Name, int(g.Value), g.Tags)
		}
	}()
	go func() {
		defer wg.Done()
		for _, t := range timers {
			r.Timer(t.Name, time.Second*time.Duration(t.Value), t.Tags)
		}
	}()
	wg.Wait()
}

func TestReporter(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	pushTestMetrics(metrics)
	time.Sleep(3 * time.Second)
	numCountersVal := atomic.LoadUint64(&numCounters)
	numGaugesVal := atomic.LoadUint64(&numGauges)
	numTimersVal := atomic.LoadUint64(&numTimers)
	var allMetricsReceived = numCountersVal == 2 && numGaugesVal == 2 && numTimersVal == 1
	if !allMetricsReceived {
		t.Fatalf("Counts incorrect :: NumCounters %v, NumGauges %v, NumTimers %v", numCountersVal, numGaugesVal, numTimersVal)
	}
}

func TestTags(t *testing.T) {
	t.Parallel()

	received := make(chan gauge, 1)
	ts := httptest.NewServer(http.HandlerFunc(relayHandler(t, received)))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Minute, nil)
	metrics.Gauge("test", 10, map[string]string{"foo": "bar"})
	metrics.Flush()
	cancel()

	first := readOne(t, received)
	if v, ok := first.Tags["foo"]; len(first.Tags) != 1 || !ok || v != "bar" {
		t.Fatalf("Expected one foo=bar tag. Got %v", first.Tags)
	}
}

func TestGlobalTags(t *testing.T) {
	t.Parallel()

	received := make(chan gauge, 1)
	ts := httptest.NewServer(http.HandlerFunc(relayHandler(t, received)))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	withTags := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Minute, map[string]string{
		"global":   "tag",
		"override": "false",
	})
	withTags.Gauge("test", 10, map[string]string{"local": "foo", "override": "bar"})
	withTags.Flush()
	cancel()

	second := readOne(t, received)
	if got := len(second.Tags); got != 3 {
		t.Fatalf("Expected 3 tags. Got: %d", got)
	}
	if got, ok := second.Tags["global"]; !ok || got != "tag" {
		t.Fatalf("Expected a global tag. Got: %v", got)
	}
	if got, ok := second.Tags["local"]; !ok || got != "foo" {
		t.Fatalf("Expected a local tag. Got: %v", got)
	}
	if got, ok := second.Tags["override"]; !ok || got != "bar" {
		t.Fatalf("Expected an overriden tag. Got: %v", got)
	}
}

func relayHandler(l testLogger, gauges chan gauge) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path[1:] != "metrics" {
			l.Logf("invalid url path: %s", r.URL.Path)
			http.NotFound(w, r)
			close(gauges)
			return
		}
		var m []gauge
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil || len(m) < 1 {
			l.Log(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			close(gauges)
			return
		}
		gauges <- m[0]
	}
}

func readOne(t *testing.T, received <-chan gauge) gauge {
	var (
		m  gauge
		ok bool
	)
	select {
	case m, ok = <-received:
		if !ok {
			t.Fatal("test server returned error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Mock server timed out waiting for a metric to be received")
	}
	return m
}

func TestFlush(t *testing.T) {
	t.Parallel()
	start := time.Now()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 100*time.Second, nil)
	pushTestMetrics(metrics)
	metrics.Flush()
	numCountersVal := atomic.LoadUint64(&numCounters)
	numGaugesVal := atomic.LoadUint64(&numGauges)
	numTimersVal := atomic.LoadUint64(&numTimers)
	allMetricsReceived := numCountersVal == 2 && numGaugesVal == 2 && numTimersVal == 1
	if !allMetricsReceived {
		t.Fatalf("Counts incorrect :: NumCountes %v, NumGauges %v, NumTimers %v", numCountersVal, numGaugesVal, numTimersVal)
	}
	if time.Since(start) > time.Second*5 {
		t.Fatal("Test took too long")
	}

}

func TestContextDone(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	var numCounters, numGauges, numTimers uint64

	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()
	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	cancel() // stop everything

	pushTestMetrics(metrics)
	metrics.Flush()
	numCountersVal := atomic.LoadUint64(&numCounters)
	numGaugesVal := atomic.LoadUint64(&numGauges)
	numTimersVal := atomic.LoadUint64(&numTimers)
	allMetricsReceived := numCountersVal == 0 && numGaugesVal == 0 && numTimersVal == 0
	if !allMetricsReceived {
		t.Fatalf("Counts incorrect :: NumCountes %v, NumGauges %v, NumTimers %v", numCountersVal, numGaugesVal, numTimersVal)
	}
}

func Test500DoesNotRetry(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var calls uint32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddUint32(&calls, 1)
		w.WriteHeader(500)
	}))
	defer ts.Close()

	reporter := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	reporter.Flush()
	pushTestMetrics(reporter)
	reporter.Flush()
	if got := atomic.LoadUint32(&calls); got != 1 {
		t.Fatalf("Expected Atlas to be called exactly once. Got %d calls", got)
	}
}

func TestAtlasTimeoutOnce(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64

	realHandler := makeMockHandler(&numCounters, &numGauges, &numTimers)
	var reqCount int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt64(&reqCount, 1) == 1 {
			select {
			case <-w.(http.CloseNotifier).CloseNotify():

			case <-time.After(20 * time.Second):

			}
		} else {
			realHandler(w, r)
		}
	}))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	pushTestMetrics(metrics)
	metrics.Flush()
	numCountersVal := atomic.LoadUint64(&numCounters)
	numGaugesVal := atomic.LoadUint64(&numGauges)
	numTimersVal := atomic.LoadUint64(&numTimers)
	allMetricsReceived := numCountersVal == 2 && numGaugesVal == 2 && numTimersVal == 1
	if !allMetricsReceived {
		t.Fatalf("Counts incorrect :: NumCountes %v, NumGauges %v, NumTimers %v", numCountersVal, numGaugesVal, numTimersVal)
	}

}

func TestAtlasTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-w.(http.CloseNotifier).CloseNotify():

		case <-time.After(20 * time.Second):
		}
	}))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	pushTestMetrics(metrics)
	metrics.Flush()
}

func TestSlowAtlas(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64

	realHandler := makeMockHandler(&numCounters, &numGauges, &numTimers)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-w.(http.CloseNotifier).CloseNotify():
		case <-time.After(3 * time.Second):
			realHandler(w, r)
		}
	}))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(t), ts.URL+"/metrics", 1*time.Second, nil)
	pushTestMetrics(metrics)
	metrics.Flush()
	numCountersVal := atomic.LoadUint64(&numCounters)
	numGaugesVal := atomic.LoadUint64(&numGauges)
	numTimersVal := atomic.LoadUint64(&numTimers)
	allMetricsReceived := numCountersVal == 2 && numGaugesVal == 2 && numTimersVal == 1
	if !allMetricsReceived {
		t.Fatalf("Counts incorrect :: NumCountes %v, NumGauges %v, NumTimers %v", numCountersVal, numGaugesVal, numTimersVal)
	}
}

func BenchmarkTimer(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(b), ts.URL+"/metrics", 1*time.Second, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.Timer("test", time.Duration(10), nil)
	}
	metrics.Flush()
}

func BenchmarkGauge(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(b), ts.URL+"/metrics", 1*time.Second, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.Gauge("test", 10, nil)
	}
	metrics.Flush()
}

func BenchmarkCounter(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	metrics := WithURLInterval(ctx, logger(b), ts.URL+"/metrics", 1*time.Second, nil)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		metrics.Counter("test", 10, nil)
	}
	metrics.Flush()
}

func BenchmarkTags(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var numCounters, numGauges, numTimers uint64
	ts := httptest.NewServer(http.HandlerFunc(makeMockHandler(&numCounters, &numGauges, &numTimers)))
	defer ts.Close()

	globalTags := map[string]string{"global": "flag"}
	metrics := WithURLInterval(ctx, logger(b), ts.URL+"/metrics", 1*time.Second, globalTags)
	tags := make(map[string]string, b.N)
	for i := 0; i < b.N; i++ {
		tags[fmt.Sprintf("key=%d", i)] = fmt.Sprintf("value=%d", i)
	}
	b.ResetTimer()
	metrics.Counter("test", 10, tags)
	metrics.Flush()
}

type testLogger interface {
	Log(...interface{})
	Logf(string, ...interface{})
}

type testLogAdapter struct {
	l testLogger
}

func logger(t testLogger) Logger {
	return &testLogAdapter{t}
}

func (l *testLogAdapter) Println(args ...interface{}) {
	l.l.Log(args...)
}

func (l *testLogAdapter) Printf(format string, args ...interface{}) {
	l.l.Logf(format, args...)
}
