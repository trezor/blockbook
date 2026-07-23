package common

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/trezor/blockbook/configs"
	yaml "gopkg.in/yaml.v3"
)

// Metrics holds prometheus collectors for various metrics collected by Blockbook.
//
// The collectors are not declared here; their name/help/type/labels/buckets live in
// configs/metrics.yaml (the single source of truth, embedded via configs.MetricsYAML).
// Each field's `metric:"<key>"` tag binds it to a definition in that file, and
// GetMetrics builds and registers the collectors by reflection at startup. Adding a
// metric means adding a struct field with a tag here and a matching entry in the YAML.
type Metrics struct {
	WebsocketRequests                 *prometheus.CounterVec   `metric:"websocket_requests"`
	WebsocketSubscribes               *prometheus.GaugeVec     `metric:"websocket_subscribes"`
	WebsocketClients                  prometheus.Gauge         `metric:"websocket_clients"`
	WebsocketReqDuration              *prometheus.HistogramVec `metric:"websocket_req_duration"`
	WebsocketChannelCloses            *prometheus.CounterVec   `metric:"websocket_channel_closes"`
	WebsocketUnknownMethods           *prometheus.CounterVec   `metric:"websocket_unknown_methods"`
	WebsocketAddrNotifications        *prometheus.CounterVec   `metric:"websocket_addr_notifications"`
	WebsocketNewBlockTxs              *prometheus.CounterVec   `metric:"websocket_new_block_txs"`
	WebsocketNewBlockTxsDuration      *prometheus.HistogramVec `metric:"websocket_new_block_txs_duration_seconds"`
	BalanceHistoryFiatDuration        *prometheus.HistogramVec `metric:"balance_history_fiat_duration_seconds"`
	BalanceHistoryFiatFallback        *prometheus.CounterVec   `metric:"balance_history_fiat_fallback_total"`
	BalanceHistoryPoints              *prometheus.HistogramVec `metric:"balance_history_points"`
	WebsocketEthReceipt               *prometheus.CounterVec   `metric:"websocket_eth_receipt"`
	WebsocketNewBlockTxsSubscriptions prometheus.Gauge         `metric:"websocket_new_block_txs_subscriptions"`
	WebsocketConnectionRequests       prometheus.Histogram     `metric:"websocket_connection_requests"`
	WebsocketConnectionRejections     *prometheus.CounterVec   `metric:"websocket_connection_rejections"`
	WebsocketUniqueIPs                prometheus.Gauge         `metric:"websocket_unique_ips"`
	WebsocketMaxConnectionsPerIP      prometheus.Gauge         `metric:"websocket_max_connections_per_ip"`
	IndexResyncDuration               prometheus.Histogram     `metric:"index_resync_duration"`
	MempoolResyncDuration             prometheus.Histogram     `metric:"mempool_resync_duration"`
	MempoolResyncThroughput           *prometheus.HistogramVec `metric:"mempool_resync_throughput_txs_per_second"`
	TxCacheEfficiency                 *prometheus.CounterVec   `metric:"txcache_efficiency"`
	RPCLatency                        *prometheus.HistogramVec `metric:"rpc_latency"`
	ChainDataFallbacks                *prometheus.CounterVec   `metric:"rpc_fallback_calls_total"`
	EthCallRequests                   *prometheus.CounterVec   `metric:"eth_call_requests"`
	EthCallErrors                     *prometheus.CounterVec   `metric:"eth_call_errors"`
	EthCallBatchSize                  prometheus.Histogram     `metric:"eth_call_batch_size"`
	EthCallContractInfo               *prometheus.CounterVec   `metric:"eth_call_contract_info_requests"`
	EthCallTokenURI                   *prometheus.CounterVec   `metric:"eth_call_token_uri_requests"`
	EthCallStakingPool                *prometheus.CounterVec   `metric:"eth_call_staking_pool_requests"`
	IndexResyncErrors                 *prometheus.CounterVec   `metric:"index_resync_errors"`
	IndexBlockNotFoundRetries         prometheus.Counter       `metric:"index_block_not_found_retries"`
	IndexReorgEvents                  *prometheus.CounterVec   `metric:"index_reorg_events"`
	IndexSyncYields                   *prometheus.CounterVec   `metric:"index_sync_yields"`
	IndexDBSize                       prometheus.Gauge         `metric:"index_db_size"`
	ExplorerViews                     *prometheus.CounterVec   `metric:"explorer_views"`
	MempoolSize                       prometheus.Gauge         `metric:"mempool_size"`
	EstimatedFee                      *prometheus.GaugeVec     `metric:"estimated_fee"`
	AvgBlockPeriod                    prometheus.Gauge         `metric:"avg_block_period"`
	SyncBlockStats                    *prometheus.GaugeVec     `metric:"sync_block_stats"`
	SyncHotnessStats                  *prometheus.GaugeVec     `metric:"sync_hotness_stats"`
	AddrContractsCacheEntries         prometheus.Gauge         `metric:"addr_contracts_cache_entries"`
	AddrContractsCacheBytes           prometheus.Gauge         `metric:"addr_contracts_cache_bytes"`
	AddrContractsCacheHits            prometheus.Counter       `metric:"addr_contracts_cache_hits_total"`
	AddrContractsCacheMisses          prometheus.Counter       `metric:"addr_contracts_cache_misses_total"`
	AddrContractsCacheFlushes         *prometheus.CounterVec   `metric:"addr_contracts_cache_flush_total"`
	DbColumnRows                      *prometheus.GaugeVec     `metric:"dbcolumn_rows"`
	DbColumnSize                      *prometheus.GaugeVec     `metric:"dbcolumn_size"`
	BlockbookAppInfo                  *prometheus.GaugeVec     `metric:"app_info"`
	BlockbookBestHeight               prometheus.Gauge         `metric:"best_height"`
	Synchronized                      prometheus.Gauge         `metric:"synchronized"`
	BackendBestHeight                 prometheus.Gauge         `metric:"backend_best_height"`
	BackendTipAgeSeconds              prometheus.Gauge         `metric:"tip_age_seconds"`
	BackendSubscriptionAgeSeconds     prometheus.Gauge         `metric:"backend_subscription_age_seconds"`
	BackendSubscriptionEvents         *prometheus.CounterVec   `metric:"backend_subscription_events"`
	AverageBlockTimeSeconds           prometheus.Gauge         `metric:"average_block_time_seconds"`
	ExplorerPendingRequests           *prometheus.GaugeVec     `metric:"explorer_pending_requests"`
	WebsocketPendingRequests          *prometheus.GaugeVec     `metric:"websocket_pending_requests"`
	XPubCacheSize                     prometheus.Gauge         `metric:"xpub_cache_size"`
	CoingeckoRequests                 *prometheus.CounterVec   `metric:"coingecko_requests"`
	CoingeckoRangeRequests            *prometheus.CounterVec   `metric:"coingecko_range_requests"`
	FiatRatesUpdateDuration           *prometheus.HistogramVec `metric:"fiat_rates_update_duration_seconds"`
	AlternativeFeeProviderRequests    *prometheus.CounterVec   `metric:"alternative_fee_provider_requests"`
	EthSyncRpcErrors                  *prometheus.CounterVec   `metric:"eth_sync_rpc_errors"`
}

// Labels represents a collection of label name -> value mappings.
type Labels = prometheus.Labels

// metricDef is one metric definition as declared in configs/metrics.yaml.
type metricDef struct {
	Name    string    `yaml:"name"`
	Type    string    `yaml:"type"`
	Help    string    `yaml:"help"`
	Labels  []string  `yaml:"labels"`
	Buckets []float64 `yaml:"buckets"`
}

// metricsConfig is the parsed configs/metrics.yaml document.
type metricsConfig struct {
	Prefix  string               `yaml:"prefix"`
	Metrics map[string]metricDef `yaml:"metrics"`
}

// GetMetrics builds and registers the prometheus collectors defined in
// configs/metrics.yaml, returning a Metrics struct whose fields point at them.
// Each struct field is matched to a YAML entry by its `metric:"<key>"` tag; the
// field's Go type must agree with the entry's declared type. Definitions and struct
// fields must be in 1:1 correspondence, otherwise GetMetrics returns an error.
func GetMetrics(coin string) (*Metrics, error) {
	var cfg metricsConfig
	if err := yaml.Unmarshal(configs.MetricsYAML, &cfg); err != nil {
		return nil, fmt.Errorf("metrics: parsing configs/metrics.yaml: %w", err)
	}

	metrics := &Metrics{}
	constLabels := Labels{"coin": coin}
	v := reflect.ValueOf(metrics).Elem()
	t := v.Type()
	used := make(map[string]bool, len(cfg.Metrics))

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		key := field.Tag.Get("metric")
		if key == "" {
			return nil, fmt.Errorf("metrics: field %s has no `metric` tag", field.Name)
		}
		def, ok := cfg.Metrics[key]
		if !ok {
			return nil, fmt.Errorf("metrics: no definition for key %q (field %s)", key, field.Name)
		}
		if used[key] {
			return nil, fmt.Errorf("metrics: duplicate `metric:%q` tag (field %s); each definition must bind to exactly one struct field", key, field.Name)
		}
		used[key] = true

		collector, err := buildCollector(field.Type, def, constLabels)
		if err != nil {
			return nil, fmt.Errorf("metrics: field %s (key %q): %w", field.Name, key, err)
		}
		if err := prometheus.Register(collector); err != nil {
			return nil, fmt.Errorf("metrics: registering %q: %w", def.Name, err)
		}
		v.Field(i).Set(reflect.ValueOf(collector))
	}

	for key := range cfg.Metrics {
		if !used[key] {
			return nil, fmt.Errorf("metrics: definition %q in configs/metrics.yaml has no corresponding struct field", key)
		}
	}

	return metrics, nil
}

// fieldTypeToMetricType maps a Metrics struct field's Go type to the metric `type`
// its configs/metrics.yaml entry must declare. This is the authoritative binding: it
// is exact (an interface field would otherwise be satisfied by several concrete
// collector types, so type-equality or assignability checks are not tight enough).
var fieldTypeToMetricType = map[string]string{
	"prometheus.Counter":       "counter",
	"*prometheus.CounterVec":   "counter_vec",
	"prometheus.Gauge":         "gauge",
	"*prometheus.GaugeVec":     "gauge_vec",
	"prometheus.Histogram":     "histogram",
	"*prometheus.HistogramVec": "histogram_vec",
}

// buildCollector constructs the prometheus collector described by def, validating that
// its declared type matches the Go type of the destination struct field and is
// internally consistent (labels for *_vec, buckets for histograms).
func buildCollector(fieldType reflect.Type, def metricDef, constLabels Labels) (prometheus.Collector, error) {
	want, ok := fieldTypeToMetricType[fieldType.String()]
	if !ok {
		return nil, fmt.Errorf("unsupported field type %s", fieldType)
	}
	if def.Type != want {
		return nil, fmt.Errorf("type %q does not match struct field type %s (expected %q)", def.Type, fieldType, want)
	}

	isVec := strings.HasSuffix(def.Type, "_vec")
	isHist := strings.HasPrefix(def.Type, "histogram")
	if isVec && len(def.Labels) == 0 {
		return nil, fmt.Errorf("type %q requires labels", def.Type)
	}
	if !isVec && len(def.Labels) > 0 {
		return nil, fmt.Errorf("type %q must not declare labels", def.Type)
	}
	if isHist && len(def.Buckets) == 0 {
		return nil, fmt.Errorf("type %q requires buckets", def.Type)
	}
	if !isHist && len(def.Buckets) > 0 {
		return nil, fmt.Errorf("type %q must not declare buckets", def.Type)
	}

	var c prometheus.Collector
	switch def.Type {
	case "counter":
		c = prometheus.NewCounter(prometheus.CounterOpts{Name: def.Name, Help: def.Help, ConstLabels: constLabels})
	case "counter_vec":
		c = prometheus.NewCounterVec(prometheus.CounterOpts{Name: def.Name, Help: def.Help, ConstLabels: constLabels}, def.Labels)
	case "gauge":
		c = prometheus.NewGauge(prometheus.GaugeOpts{Name: def.Name, Help: def.Help, ConstLabels: constLabels})
	case "gauge_vec":
		c = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: def.Name, Help: def.Help, ConstLabels: constLabels}, def.Labels)
	case "histogram":
		c = prometheus.NewHistogram(prometheus.HistogramOpts{Name: def.Name, Help: def.Help, Buckets: def.Buckets, ConstLabels: constLabels})
	case "histogram_vec":
		c = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: def.Name, Help: def.Help, Buckets: def.Buckets, ConstLabels: constLabels}, def.Labels)
	default:
		return nil, fmt.Errorf("unknown type %q", def.Type)
	}
	return c, nil
}
