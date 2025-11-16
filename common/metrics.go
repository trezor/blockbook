package common

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds prometheus collectors for various metrics collected by Blockbook
type Metrics struct {
	SocketIORequests         *prometheus.CounterVec
	SocketIOSubscribes       *prometheus.CounterVec
	SocketIOClients          prometheus.Gauge
	SocketIOReqDuration      *prometheus.HistogramVec
	WebsocketRequests        *prometheus.CounterVec
	WebsocketSubscribes      *prometheus.GaugeVec
	WebsocketClients         prometheus.Gauge
	WebsocketReqDuration     *prometheus.HistogramVec
	IndexResyncDuration      prometheus.Histogram
	MempoolResyncDuration    prometheus.Histogram
	TxCacheEfficiency        *prometheus.CounterVec
	RPCLatency               *prometheus.HistogramVec
	IndexResyncErrors        *prometheus.CounterVec
	IndexDBSize              prometheus.Gauge
	ExplorerViews            *prometheus.CounterVec
	MempoolSize              prometheus.Gauge
	EstimatedFee             *prometheus.GaugeVec
	AvgBlockPeriod           prometheus.Gauge
	DbColumnRows             *prometheus.GaugeVec
	DbColumnSize             *prometheus.GaugeVec
	BlockbookAppInfo         *prometheus.GaugeVec
	BackendBestHeight        prometheus.Gauge
	BlockbookBestHeight      prometheus.Gauge
	ExplorerPendingRequests  *prometheus.GaugeVec
	WebsocketPendingRequests *prometheus.GaugeVec
	SocketIOPendingRequests  *prometheus.GaugeVec
	XPubCacheSize            prometheus.Gauge
	CoingeckoRequests        *prometheus.CounterVec
}

// Labels represents a collection of label name -> value mappings.
type Labels = prometheus.Labels

// GetMetrics returns struct holding prometheus collectors for various metrics collected by Blockbook
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
			Help:        "Number of currently connected socketio clients",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.SocketIOReqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_socketio_req_duration",
			Help:        "Socketio request duration by method (in microseconds)",
			Buckets:     []float64{10, 100, 1_000, 10_000, 100_000, 1_000_000, 10_0000_000},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.WebsocketRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_requests",
			Help:        "Total number of websocket requests by method and status",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method", "status"},
	)
	metrics.WebsocketSubscribes = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_websocket_subscribes",
			Help:        "Number of websocket subscriptions by method",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.WebsocketClients = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_websocket_clients",
			Help:        "Number of currently connected websocket clients",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.WebsocketReqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_websocket_req_duration",
			Help:        "Websocket request duration by method (in microseconds)",
			Buckets:     []float64{10, 100, 1_000, 10_000, 100_000, 1_000_000, 10_0000_000},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.IndexResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:        "blockbook_index_resync_duration",
			Help:        "Duration of index resync operation (in milliseconds)",
			Buckets:     []float64{10, 100, 500, 1000, 2000, 5000, 10000},
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.MempoolResyncDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:        "blockbook_mempool_resync_duration",
			Help:        "Duration of mempool resync operation (in milliseconds)",
			Buckets:     []float64{10, 100, 500, 1000, 2000, 5000, 10000},
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
			Buckets:     []float64{0.1, 0.5, 1, 5, 10, 25, 50, 75, 100, 250},
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
	metrics.ExplorerViews = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_explorer_views",
			Help:        "Number of explorer views",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"action"},
	)
	metrics.MempoolSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_mempool_size",
			Help:        "Mempool size (number of transactions)",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.EstimatedFee = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_estimated_fee",
			Help:        "Estimated fee per byte (gas) for number of blocks",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"blocks", "conservative"},
	)
	metrics.AvgBlockPeriod = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_avg_block_period",
			Help:        "Average period of mining of last 100 blocks in seconds",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.DbColumnRows = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_dbcolumn_rows",
			Help:        "Number of rows in db column",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"column"},
	)
	metrics.DbColumnSize = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_dbcolumn_size",
			Help:        "Size of db column (in bytes)",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"column"},
	)
	metrics.BlockbookAppInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_app_info",
			Help:        "Information about blockbook and backend application versions",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"blockbook_version", "blockbook_commit", "blockbook_buildtime", "backend_version", "backend_subversion", "backend_protocol_version"},
	)
	metrics.BlockbookBestHeight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_best_height",
			Help:        "Block height in Blockbook",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.BackendBestHeight = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_backend_best_height",
			Help:        "Block height in backend",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.ExplorerPendingRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_explorer_pending_requests",
			Help:        "Number of unfinished requests in explorer interface",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.WebsocketPendingRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_websocket_pending_requests",
			Help:        "Number of unfinished requests in websocket interface",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.SocketIOPendingRequests = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_socketio_pending_requests",
			Help:        "Number of unfinished requests in socketio interface",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.XPubCacheSize = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_xpub_cache_size",
			Help:        "Number of cached xpubs",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.CoingeckoRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_coingecko_requests",
			Help:        "Total number of requests to coingecko",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"endpoint", "status"},
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
