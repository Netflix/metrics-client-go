package global

import (
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Netflix/metrics-client-go/metrics"
)

type metric struct {
	Name      string
	Value     int
	Type      string
	Timestamp int64
	Tags      map[string]string
}

// This test is in this package for convenience, so it can use some of the testing utils defined here. The global
// reporter is in a separate package to make it opt-in, since importing it has side effects.
func TestGlobalReporter(t *testing.T) {
	t.Parallel()
	metrics := make(chan *metric, 3)
	defer close(metrics)

	ts := httptest.NewServer(relay(metrics))
	defer ts.Close()

	SetLoggerURL(logger(t), ts.URL+"/metrics")

	c1 := &metric{Name: "c1", Value: 2, Tags: map[string]string{"appname": "testapp"}}
	Counter(c1.Name, c1.Value, c1.Tags)
	Flush()
	checkReceived(t, metrics, c1)

	g1 := &metric{Name: "g1", Value: 100, Tags: map[string]string{"appname": "testapp2", "foo": "bar"}}
	Gauge(g1.Name, g1.Value, g1.Tags)
	Flush()
	checkReceived(t, metrics, g1)

	t1 := &metric{Name: "t1", Value: 10}
	Timer(t1.Name, 10*time.Millisecond, t1.Tags)
	Flush()
	checkReceived(t, metrics, t1)
}

func checkReceived(t *testing.T, metrics <-chan *metric, expected *metric) {
	select {
	case m := <-metrics:
		if got := m.Name; got != expected.Name {
			t.Fatalf("Expected name %s. Got: %s", expected.Name, got)
		}
		if got := m.Value; got != expected.Value {
			t.Fatalf("Expected value %d. Got: %d", expected.Value, got)
		}
		if len(m.Tags) != len(expected.Tags) {
			t.Fatalf("Expected %d tags, got %d", len(expected.Tags), len(m.Tags))
		}
		for k, v := range expected.Tags {
			if got, ok := m.Tags[k]; !ok || got != v {
				t.Fatalf("Expected %s=%s. Got: %s", k, v, got)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Timed out waiting for counter metric")
	}
}

func relay(to chan<- *metric) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path[1:] != "metrics" {
			http.NotFound(w, r)
			return
		}
		var m []metric
		if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
			log.Println(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = r.Body.Close()
		to <- &m[0]
	})
}

type testLogger interface {
	Log(...interface{})
	Logf(string, ...interface{})
}

type testLogAdapter struct {
	l testLogger
}

func logger(t testLogger) metrics.Logger {
	return &testLogAdapter{t}
}

func (l *testLogAdapter) Println(args ...interface{}) {
	l.l.Log(args...)
}

func (l *testLogAdapter) Printf(format string, args ...interface{}) {
	l.l.Logf(format, args...)
}
