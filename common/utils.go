package common

import (
	"time"
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
