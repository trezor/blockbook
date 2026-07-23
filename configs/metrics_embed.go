// Package configs embeds configuration assets that ship inside the Blockbook binary.
package configs

import _ "embed"

// MetricsYAML is the embedded single source of truth for Blockbook's prometheus
// metrics (configs/metrics.yaml). It is parsed by common.GetMetrics at startup to
// initialize the metric collectors, and read directly by contrib/scripts/render_grafana.py
// to render the Grafana dashboard. Editing a metric's name, help, labels or buckets
// here updates both Blockbook and the dashboard from one place.
//
//go:embed metrics.yaml
var MetricsYAML []byte
