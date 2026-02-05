package db

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang/glog"
)

func (d *RocksDB) loadAddrContractsCacheConfigFromEnv() {
	if d.is == nil {
		return
	}
	network := strings.ToUpper(d.is.GetNetwork())
	if network == "" {
		return
	}

	if v, ok := lookupEnvInt(network+"_ADDR_CONTRACTS_CACHE_MIN_SIZE", d.addrContractsCacheMinSizeBytes); ok {
		d.addrContractsCacheMinSizeBytes = v
	}
	if v, ok := lookupEnvInt(network+"_ADDR_CONTRACTS_CACHE_ALWAYS_SIZE", d.addrContractsCacheAlwaysBytes); ok {
		d.addrContractsCacheAlwaysBytes = v
	}
	if v, ok := lookupEnvFloat(network+"_ADDR_CONTRACTS_CACHE_HOT_MIN_SCORE", d.addrContractsCacheHotMinScore); ok {
		d.addrContractsCacheHotMinScore = v
	}
	if v, ok := lookupEnvDuration(network+"_ADDR_CONTRACTS_CACHE_HOT_HALF_LIFE", d.addrContractsHotHalfLife); ok {
		d.addrContractsHotHalfLife = v
	}
	if v, ok := lookupEnvDuration(network+"_ADDR_CONTRACTS_CACHE_HOT_EVICT_AFTER", d.addrContractsHotEvictAfter); ok {
		d.addrContractsHotEvictAfter = v
	}
	if v, ok := lookupEnvDuration(network+"_ADDR_CONTRACTS_CACHE_FLUSH_IDLE", d.addrContractsCacheFlushIdle); ok {
		d.addrContractsCacheFlushIdle = v
	}
	if v, ok := lookupEnvDuration(network+"_ADDR_CONTRACTS_CACHE_FLUSH_MAX_AGE", d.addrContractsCacheFlushMaxAge); ok {
		d.addrContractsCacheFlushMaxAge = v
	}
}

func lookupEnvInt(key string, current int) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return current, false
	}
	if v, ok := parseSizeBytes(raw); ok {
		glog.Infof("address cache: env %s=%s", key, raw)
		return v, true
	}
	glog.Warningf("address cache: invalid %s=%s (expected bytes or K/M/G suffix)", key, raw)
	return current, false
}

func lookupEnvFloat(key string, current float64) (float64, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return current, false
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		glog.Warningf("address cache: invalid %s=%s (expected float)", key, raw)
		return current, false
	}
	glog.Infof("address cache: env %s=%s", key, raw)
	return v, true
}

func lookupEnvDuration(key string, current time.Duration) (time.Duration, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return current, false
	}
	if v, ok := parseDuration(raw); ok {
		glog.Infof("address cache: env %s=%s", key, raw)
		return v, true
	}
	glog.Warningf("address cache: invalid %s=%s (expected duration, e.g. 30m)", key, raw)
	return current, false
}

func parseDuration(raw string) (time.Duration, bool) {
	if d, err := time.ParseDuration(raw); err == nil {
		return d, true
	}
	if v, err := strconv.Atoi(raw); err == nil {
		return time.Duration(v) * time.Minute, true
	}
	return 0, false
}

func parseSizeBytes(raw string) (int, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	mult := int64(1)
	upper := strings.ToUpper(s)
	switch {
	case strings.HasSuffix(upper, "KIB"):
		mult = 1 << 10
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "MIB"):
		mult = 1 << 20
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "GIB"):
		mult = 1 << 30
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "TIB"):
		mult = 1 << 40
		s = s[:len(s)-3]
	case strings.HasSuffix(upper, "KB"):
		mult = 1 << 10
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "MB"):
		mult = 1 << 20
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "GB"):
		mult = 1 << 30
		s = s[:len(s)-2]
	case strings.HasSuffix(upper, "TB"):
		mult = 1 << 40
		s = s[:len(s)-2]
	default:
		last := upper[len(upper)-1]
		switch last {
		case 'K':
			mult = 1 << 10
			s = s[:len(s)-1]
		case 'M':
			mult = 1 << 20
			s = s[:len(s)-1]
		case 'G':
			mult = 1 << 30
			s = s[:len(s)-1]
		case 'T':
			mult = 1 << 40
			s = s[:len(s)-1]
		}
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	bytes := int64(v * float64(mult))
	if bytes <= 0 {
		return 0, false
	}
	if bytes > int64(^uint(0)>>1) {
		return 0, false
	}
	return int(bytes), true
}
