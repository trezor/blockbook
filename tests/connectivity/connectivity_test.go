//go:build integration

package connectivity

import "testing"

func TestBackendConnectivityEnabled(t *testing.T) {
	cases := map[string]bool{
		"":      false,
		"0":     false,
		"false": false,
		"no":    false,
		"1":     true,
		"true":  true,
		"TRUE":  true,
		"yes":   true,
		"on":    true,
		" 1 ":   true,
	}
	for value, want := range cases {
		t.Setenv(backendConnectivityEnvVar, value)
		if got := backendConnectivityEnabled(); got != want {
			t.Errorf("backendConnectivityEnabled() with %q = %v, want %v", value, got, want)
		}
	}
}
