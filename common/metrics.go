package common

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	RPCRequests           *prometheus.CounterVec
	SubscribeRequests     *prometheus.CounterVec
	Clients               *prometheus.GaugeVec
	RequestDuration       *prometheus.HistogramVec
	IndexResyncDuration   prometheus.Histogram
	MempoolResyncDuration prometheus.Histogram
	TxCacheEfficiency     *prometheus.CounterVec
	BlockChainLatency     *prometheus.HistogramVec
	IndexResyncErrors     *prometheus.CounterVec
	MempoolResyncErrors   *prometheus.CounterVec
	IndexDBSize           prometheus.Gauge
}

type Labels = prometheus.Labels

func GetMetrics() (*Metrics, error) {
	metrics := Metrics{}

	metrics.RPCRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blockbook_rpc_requests",
			Help: "Total number of RPC requests by transport, method and status",
		},
		[]string{"transport", "method", "status"},
	)
	metrics.SubscribeRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blockbook_subscribe_requests",
			Help: "Total number of subscribe requests by transport, channel and status",
		},
		[]string{"transport", "channel", "status"},
	)
	metrics.Clients = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "blockbook_clients",
			Help: "Number of currently connected clients by transport",
		},
		[]string{"transport"},
	)
	metrics.RequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blockbook_request_duration",
			Help:    "Request duration by method (in microseconds)",
			Buckets: []float64{1, 5, 10, 25, 50, 75, 100, 250},
		},
		[]string{"transport", "method"},
	)
	metrics.IndexResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "blockbook_index_resync_duration",
			Help:    "Duration of index resync operation (in milliseconds)",
			Buckets: []float64{100, 250, 500, 750, 1000, 10000, 30000, 60000},
		},
	)
	metrics.MempoolResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "blockbook_mempool_resync_duration",
			Help:    "Duration of mempool resync operation (in milliseconds)",
			Buckets: []float64{1, 5, 10, 25, 50, 75, 100, 250},
		},
	)
	metrics.TxCacheEfficiency = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blockbook_txcache_efficiency",
			Help: "Efficiency of txCache",
		},
		[]string{"status"},
	)
	metrics.BlockChainLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "blockbook_blockchain_latency",
			Help:    "Latency of blockchain RPC by coin and method (in milliseconds)",
			Buckets: []float64{1, 5, 10, 25, 50, 75, 100, 250},
		},
		[]string{"coin", "method"},
	)
	metrics.IndexResyncErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blockbook_index_resync_errors",
			Help: "Number of errors of index resync operation",
		},
		[]string{"error"},
	)
	metrics.MempoolResyncErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "blockbook_mempool_resync_errors",
			Help: "Number of errors of mempool resync operation",
		},
		[]string{"error"},
	)
	metrics.IndexDBSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "blockbook_index_db_size",
			Help: "Size of index database (in bytes)",
		},
	)

	v := reflect.ValueOf(metrics)
	for i := 0; i < v.NumField(); i++ {
		c := v.Field(i).Interface().(prometheus.Collector)
		err := prometheus.Register(c)
		if err != nil {
			return nil, err
		}
	}

	return &metrics, nil
}
