//go:build unittest

package server

import (
	"strings"
	"testing"

	"github.com/trezor/blockbook/api"
	"github.com/trezor/blockbook/bchain/coins/eth"
)

func TestUnmarshalAddressesReturnsPublicAPIError(t *testing.T) {
	s := &WebsocketServer{
		chainParser: eth.NewEthereumParser(0, false),
	}

	_, _, err := s.unmarshalAddresses([]byte(`{"addresses":[""]}`))
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*api.APIError)
	if !ok {
		t.Fatalf("expected *api.APIError, got %T", err)
	}
	if !apiErr.Public {
		t.Fatal("expected public api error")
	}
	if !strings.Contains(apiErr.Error(), "Address missing") {
		t.Fatalf("unexpected error message %q", apiErr.Error())
	}
}
