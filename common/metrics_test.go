//go:build unittest

package common

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/trezor/blockbook/configs"
	yaml "gopkg.in/yaml.v3"
)

var prometheusRegistryMu sync.Mutex

func useTestPrometheusRegistry(t *testing.T) {
	t.Helper()

	prometheusRegistryMu.Lock()
	oldRegisterer := prometheus.DefaultRegisterer
	oldGatherer := prometheus.DefaultGatherer
	registry := prometheus.NewRegistry()
	prometheus.DefaultRegisterer = registry
	prometheus.DefaultGatherer = registry

	t.Cleanup(func() {
		prometheus.DefaultRegisterer = oldRegisterer
		prometheus.DefaultGatherer = oldGatherer
		prometheusRegistryMu.Unlock()
	})
}

// TestGetMetrics verifies that every field of the Metrics struct is bound to a
// definition in configs/metrics.yaml, of a matching type, and that the resulting
// collectors are all constructed (non-nil) after loading.
func TestGetMetrics(t *testing.T) {
	useTestPrometheusRegistry(t)

	m, err := GetMetrics("metrics_unittest")
	if err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	v := reflect.ValueOf(m).Elem()
	tp := v.Type()
	for i := 0; i < tp.NumField(); i++ {
		if v.Field(i).IsNil() {
			t.Errorf("field %s was not initialized from configs/metrics.yaml", tp.Field(i).Name)
		}
		if tag := tp.Field(i).Tag.Get("metric"); tag == "" {
			t.Errorf("field %s is missing its `metric` tag", tp.Field(i).Name)
		}
	}
}

// TestMetricsYAMLInvariants checks the embedded single-source-of-truth file for the
// invariants the loader and the Grafana renderer both rely on: 1:1 correspondence with
// the struct, unique prometheus names, the common prefix, and key/name being distinct
// enough that the stable-key indirection holds (key never carries the prefix).
func TestMetricsYAMLInvariants(t *testing.T) {
	var cfg metricsConfig
	if err := yaml.Unmarshal(configs.MetricsYAML, &cfg); err != nil {
		t.Fatalf("parsing embedded metrics.yaml: %v", err)
	}
	if cfg.Prefix == "" {
		t.Fatal("metrics.yaml: prefix must be set")
	}

	numFields := reflect.TypeOf(Metrics{}).NumField()
	if len(cfg.Metrics) != numFields {
		t.Errorf("metrics.yaml has %d entries but Metrics struct has %d fields (must be 1:1)", len(cfg.Metrics), numFields)
	}

	names := make(map[string]string, len(cfg.Metrics))
	for key, def := range cfg.Metrics {
		if !strings.HasPrefix(def.Name, cfg.Prefix) {
			t.Errorf("metric %q: name %q does not start with prefix %q", key, def.Name, cfg.Prefix)
		}
		if strings.HasPrefix(key, cfg.Prefix) {
			t.Errorf("metric %q: stable key must not carry the %q prefix", key, cfg.Prefix)
		}
		if prev, dup := names[def.Name]; dup {
			t.Errorf("duplicate prometheus name %q (keys %q and %q)", def.Name, prev, key)
		}
		names[def.Name] = key
	}
}

// TestGoRuntimeMetricsExported pins the runtime/metrics families the Grafana system
// panels query (GC CPU share, live/goal heap) and that swapping the Go collector keeps
// the memstats gauges older panels use while leaving the noisy size histograms out.
func TestGoRuntimeMetricsExported(t *testing.T) {
	useTestPrometheusRegistry(t)
	// The fresh test registry has no stock Go collector; the production registry does.
	prometheus.MustRegister(collectors.NewGoCollector())

	if _, err := GetMetrics("metrics_unittest_go_runtime"); err != nil {
		t.Fatalf("GetMetrics: %v", err)
	}
	// A second registration in the same process (multi-coin test binaries) must be a no-op.
	if err := registerGoRuntimeMetrics(); err != nil {
		t.Fatalf("second registerGoRuntimeMetrics: %v", err)
	}

	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	names := make(map[string]bool, len(families))
	for _, f := range families {
		names[f.GetName()] = true
	}
	for _, want := range []string{
		"go_cpu_classes_gc_total_cpu_seconds_total",
		"go_cpu_classes_total_cpu_seconds_total",
		"go_cpu_classes_idle_cpu_seconds_total",
		"go_gc_heap_live_bytes",
		"go_gc_heap_goal_bytes",
		"go_memstats_alloc_bytes",
		"go_gc_duration_seconds",
	} {
		if !names[want] {
			t.Errorf("metric family %s not exported", want)
		}
	}
	for _, unwanted := range []string{"go_gc_heap_allocs_by_size_bytes", "go_gc_heap_frees_by_size_bytes"} {
		if names[unwanted] {
			t.Errorf("metric family %s should be excluded", unwanted)
		}
	}
}
