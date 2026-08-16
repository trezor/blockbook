package fiat

import "github.com/trezor/blockbook/db"

const maxHistoricalBootstrapAttempts = 3

// historicalBootstrapInProgress returns whether historical fiat bootstrap is in progress.
// stateFound indicates if the persisted bootstrap marker already exists.
func historicalBootstrapInProgress(database *db.RocksDB) (inProgress bool, stateFound bool, err error) {
	bootstrapComplete, bootstrapStateFound, err := database.FiatRatesGetHistoricalBootstrapComplete()
	if err != nil {
		return false, false, err
	}
	if bootstrapStateFound {
		return !bootstrapComplete, true, nil
	}
	lastFiatTicker, err := database.FiatRatesFindLastTicker("", "")
	if err != nil {
		return false, false, err
	}
	return lastFiatTicker == nil, false, nil
}

// ensureHistoricalBootstrapState ensures persisted bootstrap marker exists and returns current in-progress state.
func ensureHistoricalBootstrapState(database *db.RocksDB) (inProgress bool, err error) {
	inProgress, stateFound, err := historicalBootstrapInProgress(database)
	if err != nil {
		return false, err
	}
	if !stateFound {
		if err := database.FiatRatesSetHistoricalBootstrapComplete(!inProgress); err != nil {
			return false, err
		}
		if err := database.FiatRatesSetHistoricalBootstrapAttempts(0); err != nil {
			return false, err
		}
	}
	return inProgress, nil
}

// registerHistoricalBootstrapAttemptFailure increases failed bootstrap attempt count.
// Once the limit is reached, bootstrap is finalized to stop further bootstrap retries.
func registerHistoricalBootstrapAttemptFailure(database *db.RocksDB) (attempts int, exhausted bool, err error) {
	attempts, _, err = database.FiatRatesGetHistoricalBootstrapAttempts()
	if err != nil {
		return 0, false, err
	}
	attempts++
	if err := database.FiatRatesSetHistoricalBootstrapAttempts(attempts); err != nil {
		return 0, false, err
	}
	if attempts < maxHistoricalBootstrapAttempts {
		return attempts, false, nil
	}
	if err := database.FiatRatesSetHistoricalBootstrapComplete(true); err != nil {
		return attempts, false, err
	}
	if err := database.FiatRatesSetHistoricalBootstrapAttempts(0); err != nil {
		return attempts, false, err
	}
	return attempts, true, nil
}

func resetHistoricalBootstrapAttempts(database *db.RocksDB) error {
	return database.FiatRatesSetHistoricalBootstrapAttempts(0)
}
