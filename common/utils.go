package common

import (
	"encoding/json"
	"io"
	"math"
	"runtime/debug"
	"time"

	"github.com/golang/glog"
	"github.com/juju/errors"
)

// TickAndDebounce calls function f on trigger channel or with tickTime period (whatever is sooner) with debounce
func TickAndDebounce(tickTime time.Duration, debounceTime time.Duration, trigger chan struct{}, f func()) {
	timer := time.NewTimer(tickTime)
	var firstDebounce time.Time
Loop:
	for {
		select {
		case _, ok := <-trigger:
			if !timer.Stop() {
				<-timer.C
			}
			// exit loop on closed input channel
			if !ok {
				break Loop
			}
			if firstDebounce.IsZero() {
				firstDebounce = time.Now()
			}
			// debounce for up to debounceTime period
			// afterwards execute immediately
			if firstDebounce.Add(debounceTime).After(time.Now()) {
				timer.Reset(debounceTime)
			} else {
				timer.Reset(0)
			}
		case <-timer.C:
			// do the action, if not in shutdown, then start the loop again
			if !IsInShutdown() {
				f()
			}
			timer.Reset(tickTime)
			firstDebounce = time.Time{}
		}
	}
}

// SafeDecodeResponseFromReader reads from io.ReadCloser safely, with recovery from panic
func SafeDecodeResponseFromReader(body io.ReadCloser, res interface{}) (err error) {
	var data []byte
	defer func() {
		if r := recover(); r != nil {
			glog.Error("unmarshal json recovered from panic: ", r, "; data: ", string(data))
			debug.PrintStack()
			if len(data) > 0 && len(data) < 2048 {
				err = errors.Errorf("Error: %v", string(data))
			} else {
				err = errors.New("Internal error")
			}
		}
	}()
	data, err = io.ReadAll(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &res)
}

// RoundToSignificantDigits rounds a float64 number `n` to the specified number of significant figures `digits`.
// For example, RoundToSignificantDigits(1234, 3) returns 1230
//
// This function works by shifting the number's decimal point to make the desired significant figures
// into whole numbers, rounding, and then shifting back.
//
// Example for n = 1234, digits = 3:
//
//	log10(1234) ≈ 3.09 → ceil = 4
//	power = 3 - 4 = -1
//	magnitude = 10^-1 = 0.1
//	n * magnitude = 1234 * 0.1 = 123.4
//	round(123.4) = 123
//	123 / 0.1 = 1230
//
// Returns the number rounded to the desired number of significant figures.
func RoundToSignificantDigits(n float64, digits int) float64 {
	if n == 0 {
		return 0
	}

	// Step 1: Compute how many digits are before the decimal point.
	// For 1234 → log10(1234) ≈ 3.09 → ceil = 4
	d := math.Ceil(math.Log10(math.Abs(n)))

	// Step 2: Calculate how much we need to shift the number to bring
	// the significant digits into the integer part.
	// For digits=3 and d=4 → power = -1
	power := digits - int(d)

	// Step 3: Compute 10^power to scale the number
	// 10^-1 = 0.1
	magnitude := math.Pow(10, float64(power))

	// Step 4: Scale, round, and scale back
	// 1234 * 0.1 = 123.4 → round = 123 → 123 / 0.1 = 1230
	return math.Round(n*magnitude) / magnitude
}
