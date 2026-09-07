//go:build unittest

package api

import "testing"

func TestRedactAPIError(t *testing.T) {
	t.Run("redacts and keeps the original intact", func(t *testing.T) {
		original := &APIError{Text: `Post "https://relay.example.com/v1/SECRET": EOF`, Public: true}
		redacted := RedactAPIError(original)
		if redacted.Text != `Post "https://relay.example.com": EOF` {
			t.Errorf("redacted text = %q", redacted.Text)
		}
		if !redacted.Public {
			t.Error("Public flag not carried over")
		}
		if original.Text != `Post "https://relay.example.com/v1/SECRET": EOF` {
			t.Errorf("original error was mutated: %q", original.Text)
		}
	})

	t.Run("returns the same error when there is nothing to redact", func(t *testing.T) {
		original := &APIError{Text: "nonce too low", Public: true}
		if RedactAPIError(original) != original {
			t.Error("expected the original error to be returned unchanged")
		}
	})

	t.Run("nil", func(t *testing.T) {
		if RedactAPIError(nil) != nil {
			t.Error("expected nil")
		}
	})
}
