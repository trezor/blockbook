package api

import (
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/trezor/blockbook/common"
)

func normalizeBalanceHistoryPathLabel(pathLabel string) string {
	if pathLabel == "" {
		return "unknown"
	}
	return pathLabel
}

func normalizeCurrenciesToLowercase(currencies []string) []string {
	currenciesLowercase := make([]string, len(currencies))
	for i := range currencies {
		currenciesLowercase[i] = strings.ToLower(currencies[i])
	}
	return currenciesLowercase
}

func buildBalanceHistoryTimestamps(histories BalanceHistories) []int64 {
	timestamps := make([]int64, len(histories))
	for i := range histories {
		timestamps[i] = int64(histories[i].Time)
	}
	return timestamps
}

func applyTickerToBalanceHistory(bh *BalanceHistory, ticker *common.CurrencyRatesTicker, currenciesLowercase []string) {
	if ticker == nil {
		return
	}
	if len(currenciesLowercase) == 0 {
		bh.FiatRates = ticker.Rates
		return
	}
	rates := make(map[string]float32, len(currenciesLowercase))
	for _, currency := range currenciesLowercase {
		if rate, found := ticker.Rates[currency]; found {
			rates[currency] = rate
		} else {
			rates[currency] = -1
		}
	}
	bh.FiatRates = rates
}

func classifyBalanceHistoryBatchLookup(expectedLen int, tickers *[]*common.CurrencyRatesTicker, err error) (bool, string, int) {
	batchFetchValid := err == nil && tickers != nil && len(*tickers) == expectedLen
	if batchFetchValid {
		return true, "", len(*tickers)
	}
	reason := "batch_error"
	returnedTickers := -1
	if err == nil {
		if tickers == nil {
			reason = "empty_result"
		} else {
			returnedTickers = len(*tickers)
			reason = "len_mismatch"
		}
	}
	return false, reason, returnedTickers
}

type balanceHistoryFallbackStats struct {
	errorCount           int
	nilResultCount       int
	emptyResultCount     int
	firstFailedSet       bool
	firstFailedTimestamp int64
	firstFailedErr       error
}

func (s *balanceHistoryFallbackStats) recordFailure(ts int64, pointErr error, pointTickers *[]*common.CurrencyRatesTicker) {
	if !s.firstFailedSet {
		s.firstFailedSet = true
		s.firstFailedTimestamp = ts
		s.firstFailedErr = pointErr
	}
	if pointErr != nil {
		s.errorCount++
	} else if pointTickers == nil {
		s.nilResultCount++
	} else {
		s.emptyResultCount++
	}
}

func (s *balanceHistoryFallbackStats) failedTotal() int {
	return s.errorCount + s.nilResultCount + s.emptyResultCount
}

func (s *balanceHistoryFallbackStats) status() string {
	if s.failedTotal() > 0 {
		return "err"
	}
	return "ok"
}

func (s *balanceHistoryFallbackStats) logSummary(total int) {
	if s.failedTotal() == 0 {
		return
	}
	glog.Errorf(
		"Error finding fallback tickers for %d/%d timestamps (errors=%d nil_results=%d empty_results=%d first_failed_at=%d first_error=%v)",
		s.failedTotal(),
		total,
		s.errorCount,
		s.nilResultCount,
		s.emptyResultCount,
		s.firstFailedTimestamp,
		s.firstFailedErr,
	)
}

func (w *Worker) observeBalanceHistoryFiatDuration(pathLabel, mode, status string, startedAt time.Time) {
	if w.metrics == nil {
		return
	}
	w.metrics.BalanceHistoryFiatDuration.With(common.Labels{
		"path":   pathLabel,
		"mode":   mode,
		"status": status,
	}).Observe(time.Since(startedAt).Seconds())
}

func (w *Worker) incrementBalanceHistoryFiatFallback(pathLabel, reason string) {
	if w.metrics == nil {
		return
	}
	w.metrics.BalanceHistoryFiatFallback.With(common.Labels{
		"path":   pathLabel,
		"reason": reason,
	}).Inc()
}

func (w *Worker) lookupBalanceHistoryBatchTickers(timestamps []int64, pathLabel string, expectedLen int) (*[]*common.CurrencyRatesTicker, bool, string, int, error) {
	batchStarted := time.Now()
	tickers, err := getTickersForTimestamps(w.fiatRates, timestamps, "", "")
	batchFetchValid, reason, returnedTickers := classifyBalanceHistoryBatchLookup(expectedLen, tickers, err)
	status := "ok"
	if !batchFetchValid {
		status = "err"
	}
	w.observeBalanceHistoryFiatDuration(pathLabel, "batch", status, batchStarted)
	return tickers, batchFetchValid, reason, returnedTickers, err
}

func applyBatchTickersToBalanceHistories(histories BalanceHistories, tickers *[]*common.CurrencyRatesTicker, currenciesLowercase []string) {
	for i := range histories {
		applyTickerToBalanceHistory(&histories[i], (*tickers)[i], currenciesLowercase)
	}
}

func (w *Worker) applyFallbackTickersToBalanceHistories(histories BalanceHistories, currenciesLowercase []string, pathLabel string) {
	// Fallback to per-point lookup to preserve original behavior on partial failures.
	fallbackStarted := time.Now()
	stats := balanceHistoryFallbackStats{}
	for i := range histories {
		bh := &histories[i]
		pointTickers, pointErr := getTickersForTimestamps(w.fiatRates, []int64{int64(bh.Time)}, "", "")
		if pointErr != nil || pointTickers == nil || len(*pointTickers) == 0 {
			stats.recordFailure(int64(bh.Time), pointErr, pointTickers)
			continue
		}
		applyTickerToBalanceHistory(bh, (*pointTickers)[0], currenciesLowercase)
	}
	stats.logSummary(len(histories))
	w.observeBalanceHistoryFiatDuration(pathLabel, "fallback", stats.status(), fallbackStarted)
}

func (w *Worker) setFiatRateToBalanceHistories(histories BalanceHistories, currencies []string, pathLabel string) error {
	if len(histories) == 0 || w.fiatRates == nil || !w.fiatRates.Enabled {
		return nil
	}
	pathLabel = normalizeBalanceHistoryPathLabel(pathLabel)
	currenciesLowercase := normalizeCurrenciesToLowercase(currencies)
	timestamps := buildBalanceHistoryTimestamps(histories)
	tickers, batchFetchValid, reason, returnedTickers, err := w.lookupBalanceHistoryBatchTickers(timestamps, pathLabel, len(histories))
	if batchFetchValid {
		applyBatchTickersToBalanceHistories(histories, tickers, currenciesLowercase)
		return nil
	}
	glog.Errorf("Error finding tickers for %d timestamps (returned %d, reason %s). Error: %v", len(timestamps), returnedTickers, reason, err)
	w.incrementBalanceHistoryFiatFallback(pathLabel, reason)
	w.applyFallbackTickersToBalanceHistories(histories, currenciesLowercase, pathLabel)
	return nil
}
