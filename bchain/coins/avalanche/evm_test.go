package avalanche

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/rpc"
	"github.com/trezor/blockbook/bchain"
)

type testAvalancheRPCService struct{}

func (s *testAvalancheRPCService) Unfinalized() (string, error) {
	return "", errors.New("cannot query unfinalized data")
}

func (s *testAvalancheRPCService) OtherError() (string, error) {
	return "", errors.New("other failure")
}

func (s *testAvalancheRPCService) Success() (string, error) {
	return "ok", nil
}

func newTestAvalancheRPCClient(t *testing.T) *AvalancheRPCClient {
	t.Helper()

	server := rpc.NewServer()
	if err := server.RegisterName("test", &testAvalancheRPCService{}); err != nil {
		t.Fatalf("RegisterName() error = %v", err)
	}
	client := rpc.DialInProc(server)
	t.Cleanup(func() {
		client.Close()
		server.Stop()
	})

	return &AvalancheRPCClient{Client: client}
}

func TestAvalancheRPCClientCallContextMapsUnfinalizedDataToBlockNotFound(t *testing.T) {
	client := newTestAvalancheRPCClient(t)

	var result string
	err := client.CallContext(context.Background(), &result, "test_unfinalized")
	if !errors.Is(err, bchain.ErrBlockNotFound) {
		t.Fatalf("CallContext() error = %v, want ErrBlockNotFound", err)
	}
}

func TestAvalancheRPCClientCallContextReturnsOtherErrors(t *testing.T) {
	client := newTestAvalancheRPCClient(t)

	var result string
	err := client.CallContext(context.Background(), &result, "test_otherError")
	if err == nil || !strings.Contains(err.Error(), "other failure") {
		t.Fatalf("CallContext() error = %v, want other failure", err)
	}
}

func TestAvalancheRPCClientCallContextReturnsResult(t *testing.T) {
	client := newTestAvalancheRPCClient(t)

	var result string
	if err := client.CallContext(context.Background(), &result, "test_success"); err != nil {
		t.Fatalf("CallContext() error = %v", err)
	}
	if result != "ok" {
		t.Fatalf("result = %q, want %q", result, "ok")
	}
}
