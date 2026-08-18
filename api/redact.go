package api

import "github.com/trezor/blockbook/common"

// RedactAPIError returns a copy of e with the URLs in its text reduced to scheme and host (see
// common.RedactURLs), leaving the original error untouched so a caller that logs it still gets the
// full URL. Returns e itself when nothing changes.
func RedactAPIError(e *APIError) *APIError {
	if e == nil {
		return nil
	}
	text := common.RedactURLs(e.Text)
	if text == e.Text {
		return e
	}
	return &APIError{Text: text, Public: e.Public}
}
