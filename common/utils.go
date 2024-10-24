package common

import (
	"encoding/json"
	"io"
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
