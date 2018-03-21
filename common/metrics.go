package common

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

type Metrics struct {
	SocketIORequests      *prometheus.CounterVec
	SocketIOSubscribes    *prometheus.CounterVec
	SocketIOClients       prometheus.Gauge
	SocketIOReqDuration   *prometheus.HistogramVec
	IndexResyncDuration   prometheus.Histogram
	MempoolResyncDuration prometheus.Histogram
	TxCacheEfficiency     *prometheus.CounterVec
	RPCLatency            *prometheus.HistogramVec
	IndexResyncErrors     *prometheus.CounterVec
	IndexDBSize           prometheus.Gauge
}

type Labels = prometheus.Labels

func GetMetrics(coin string) (*Metrics, error) {
	metrics := Metrics{}

	metrics.SocketIORequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_socketio_requests",
			Help:        "Total number of socketio requests by method and status",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method", "status"},
	)
	metrics.SocketIOSubscribes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_socketio_subscribes",
			Help:        "Total number of socketio subscribes by channel and status",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"channel", "status"},
	)
	metrics.SocketIOClients = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_socketio_clients",
			Help:        "Number of currently connected clients",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.SocketIOReqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_socketio_req_duration",
			Help:        "Socketio request duration by method (in microseconds)",
			Buckets:     []float64{1, 5, 10, 25, 50, 75, 100, 250},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.IndexResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:        "blockbook_index_resync_duration",
			Help:        "Duration of index resync operation (in milliseconds)",
			Buckets:     []float64{100, 250, 500, 750, 1000, 10000, 30000, 60000},
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.MempoolResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:        "blockbook_mempool_resync_duration",
			Help:        "Duration of mempool resync operation (in milliseconds)",
			Buckets:     []float64{1, 5, 10, 25, 50, 75, 100, 250},
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.TxCacheEfficiency = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_txcache_efficiency",
			Help:        "Efficiency of txCache",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"status"},
	)
	metrics.RPCLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_rpc_latency",
			Help:        "Latency of blockchain RPC by method (in milliseconds)",
			Buckets:     []float64{1, 5, 10, 25, 50, 75, 100, 250},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method", "error"},
	)
	metrics.IndexResyncErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_index_resync_errors",
			Help:        "Number of errors of index resync operation",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"error"},
	)
	metrics.IndexDBSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_index_db_size",
			Help:        "Size of index database (in bytes)",
			ConstLabels: Labels{"coin": coin},
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
