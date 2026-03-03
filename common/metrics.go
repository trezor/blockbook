package common

import (
	"reflect"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds prometheus collectors for various metrics collected by Blockbook
type Metrics struct {
	SocketIORequests                  *prometheus.CounterVec
	SocketIOSubscribes                *prometheus.CounterVec
	SocketIOClients                   prometheus.Gauge
	SocketIOReqDuration               *prometheus.HistogramVec
	WebsocketRequests                 *prometheus.CounterVec
	WebsocketSubscribes               *prometheus.GaugeVec
	WebsocketClients                  prometheus.Gauge
	WebsocketReqDuration              *prometheus.HistogramVec
	WebsocketChannelCloses            *prometheus.CounterVec
	WebsocketUnknownMethods           *prometheus.CounterVec
	WebsocketAddrNotifications        *prometheus.CounterVec
	WebsocketNewBlockTxs              *prometheus.CounterVec
	WebsocketNewBlockTxsDuration      *prometheus.HistogramVec
	BalanceHistoryFiatDuration        *prometheus.HistogramVec
	BalanceHistoryFiatFallback        *prometheus.CounterVec
	BalanceHistoryPoints              *prometheus.HistogramVec
	WebsocketEthReceipt               *prometheus.CounterVec
	WebsocketNewBlockTxsSubscriptions prometheus.Gauge
	IndexResyncDuration               prometheus.Histogram
	MempoolResyncDuration             prometheus.Histogram
	MempoolResyncThroughput           *prometheus.HistogramVec
	TxCacheEfficiency                 *prometheus.CounterVec
	RPCLatency                        *prometheus.HistogramVec
	ChainDataFallbacks                *prometheus.CounterVec
	EthCallRequests                   *prometheus.CounterVec
	EthCallErrors                     *prometheus.CounterVec
	EthCallBatchSize                  prometheus.Histogram
	EthCallContractInfo               *prometheus.CounterVec
	EthCallTokenURI                   *prometheus.CounterVec
	EthCallStakingPool                *prometheus.CounterVec
	IndexResyncErrors                 *prometheus.CounterVec
	IndexDBSize                       prometheus.Gauge
	ExplorerViews                     *prometheus.CounterVec
	MempoolSize                       prometheus.Gauge
	EstimatedFee                      *prometheus.GaugeVec
	AvgBlockPeriod                    prometheus.Gauge
	SyncBlockStats                    *prometheus.GaugeVec
	SyncHotnessStats                  *prometheus.GaugeVec
	AddrContractsCacheEntries         prometheus.Gauge
	AddrContractsCacheBytes           prometheus.Gauge
	AddrContractsCacheHits            prometheus.Counter
	AddrContractsCacheMisses          prometheus.Counter
	AddrContractsCacheFlushes         *prometheus.CounterVec
	DbColumnRows                      *prometheus.GaugeVec
	DbColumnSize                      *prometheus.GaugeVec
	BlockbookAppInfo                  *prometheus.GaugeVec
	BackendBestHeight                 prometheus.Gauge
	BlockbookBestHeight               prometheus.Gauge
	ExplorerPendingRequests           *prometheus.GaugeVec
	WebsocketPendingRequests          *prometheus.GaugeVec
	SocketIOPendingRequests           *prometheus.GaugeVec
	XPubCacheSize                     prometheus.Gauge
	CoingeckoRequests                 *prometheus.CounterVec
	CoingeckoRangeRequests            *prometheus.CounterVec
	FiatRatesUpdateDuration           *prometheus.HistogramVec
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
	metrics.WebsocketChannelCloses = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_channel_closes",
			Help:        "Total number of websocket channel closes by reason",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"reason"},
	)
	metrics.WebsocketUnknownMethods = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_unknown_methods",
			Help:        "Total number of websocket requests with unknown method",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.WebsocketAddrNotifications = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_addr_notifications",
			Help:        "Total number of per-address websocket tx notifications by source",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"source"},
	)
	metrics.WebsocketNewBlockTxs = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_new_block_txs",
			Help:        "Total number of websocket newBlockTxs events by stage and status",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"stage", "status"},
	)
	metrics.WebsocketNewBlockTxsDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_websocket_new_block_txs_duration_seconds",
			Help:        "Duration of websocket newBlockTxs processing stages in seconds",
			Buckets:     []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"stage"},
	)
	metrics.BalanceHistoryFiatDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_balance_history_fiat_duration_seconds",
			Help:        "Duration of balance history fiat lookup stage by request path and mode",
			Buckets:     []float64{0.0005, 0.001, 0.005, 0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"path", "mode", "status"},
	)
	metrics.BalanceHistoryFiatFallback = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_balance_history_fiat_fallback_total",
			Help:        "Number of balance history fiat lookup fallbacks by path and reason",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"path", "reason"},
	)
	metrics.BalanceHistoryPoints = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_balance_history_points",
			Help:        "Number of output points in balance history responses by request path",
			Buckets:     []float64{1, 2, 5, 10, 20, 40, 80, 160, 320, 640, 1280},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"path"},
	)
	metrics.WebsocketEthReceipt = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_websocket_eth_receipt",
			Help:        "Total number of websocket Ethereum receipt enrichment outcomes",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"status"},
	)
	metrics.WebsocketNewBlockTxsSubscriptions = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_websocket_new_block_txs_subscriptions",
			Help:        "Number of websocket address subscriptions with newBlockTxs enabled",
			ConstLabels: Labels{"coin": coin},
		},
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
	metrics.MempoolResyncThroughput = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_mempool_resync_throughput_txs_per_second",
			Help:        "Effective mempool resync throughput in transactions per second",
			Buckets:     []float64{0.1, 0.5, 1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"chain", "status"},
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
	metrics.ChainDataFallbacks = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_rpc_fallback_calls_total",
			Help:        "Total number of chain data fallback path uses by component and reason",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"component", "reason"},
	)
	metrics.EthCallRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_eth_call_requests",
			Help:        "Total number of eth_call requests by mode",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"mode"},
	)
	metrics.EthCallErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_eth_call_errors",
			Help:        "Total number of eth_call errors by mode and type",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"mode", "type"},
	)
	metrics.EthCallBatchSize = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:        "blockbook_eth_call_batch_size",
			Help:        "Number of eth_call items per batch request",
			Buckets:     []float64{1, 2, 5, 10, 20, 50, 100, 200},
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.EthCallContractInfo = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_eth_call_contract_info_requests",
			Help:        "Total number of eth_call requests for contract info fields",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"field"},
	)
	metrics.EthCallTokenURI = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_eth_call_token_uri_requests",
			Help:        "Total number of eth_call requests for token URI lookups",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"method"},
	)
	metrics.EthCallStakingPool = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_eth_call_staking_pool_requests",
			Help:        "Total number of eth_call requests for staking pool lookups",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"field"},
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
	metrics.SyncBlockStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_sync_block_stats",
			Help:        "Per-interval block stats for bulk sync and per-block stats at chain tip",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"scope", "kind"},
	)
	metrics.SyncHotnessStats = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name:        "blockbook_sync_hotness_stats",
			Help:        "Hot address stats for bulk sync intervals and per-block chain tip processing (Ethereum-type only)",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"scope", "kind"},
	)
	metrics.AddrContractsCacheEntries = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_addr_contracts_cache_entries",
			Help:        "Number of cached addressContracts entries",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.AddrContractsCacheBytes = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name:        "blockbook_addr_contracts_cache_bytes",
			Help:        "Estimated bytes in the addressContracts cache",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.AddrContractsCacheHits = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "blockbook_addr_contracts_cache_hits_total",
			Help:        "Total number of addressContracts cache hits",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.AddrContractsCacheMisses = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name:        "blockbook_addr_contracts_cache_misses_total",
			Help:        "Total number of addressContracts cache misses",
			ConstLabels: Labels{"coin": coin},
		},
	)
	metrics.AddrContractsCacheFlushes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_addr_contracts_cache_flush_total",
			Help:        "Total number of addressContracts cache flushes by reason",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"reason"},
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
	metrics.CoingeckoRangeRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name:        "blockbook_coingecko_range_requests",
			Help:        "Total number of coingecko range queries by range kind",
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"range"},
	)
	metrics.FiatRatesUpdateDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:        "blockbook_fiat_rates_update_duration_seconds",
			Help:        "Duration of fiat rates downloader update stages in seconds",
			Buckets:     []float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30, 60, 120, 300},
			ConstLabels: Labels{"coin": coin},
		},
		[]string{"stage", "status"},
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
