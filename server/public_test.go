// build unittest

package server

import (
	"blockbook/bchain"
	"blockbook/bchain/coins/btc"
	"blockbook/common"
	"blockbook/db"
	"blockbook/tests/dbtestdata"
	"io/ioutil"
	"net"
	"os"
	"strconv"
	"testing"

	"github.com/golang/glog"
	"github.com/jakm/btcutil/chaincfg"
)

func TestMain(m *testing.M) {
	// set the current directory to blockbook root so that ./static/ works
	if err := os.Chdir(".."); err != nil {
		glog.Fatal("Chdir error:", err)
	}
	c := m.Run()
	chaincfg.ResetParams()
	os.Exit(c)
}

func setupRocksDB(t *testing.T, parser bchain.BlockChainParser) (*db.RocksDB, *common.InternalState, string) {
	tmp, err := ioutil.TempDir("", "testdb")
	if err != nil {
		t.Fatal(err)
	}
	d, err := db.NewRocksDB(tmp, 100000, -1, parser, nil)
	if err != nil {
		t.Fatal(err)
	}
	is, err := d.LoadInternalState("btc-testnet")
	if err != nil {
		t.Fatal(err)
	}
	d.SetInternalState(is)
	// import data
	if err := d.ConnectBlock(dbtestdata.GetTestUTXOBlock1(parser)); err != nil {
		t.Fatal(err)
	}
	if err := d.ConnectBlock(dbtestdata.GetTestUTXOBlock2(parser)); err != nil {
		t.Fatal(err)
	}
	return d, is, tmp
}

// getFreePort asks the kernel for a free open port that is ready to use
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func setupPublicHTTPServer(t *testing.T) (*PublicServer, string, string) {
	parser := btc.NewBitcoinParser(
		btc.GetChainParams("test"),
		&btc.Configuration{BlockAddressesToKeep: 1})

	db, is, path := setupRocksDB(t, parser)

	port, err := getFreePort()
	if err != nil {
		t.Fatal(err)
	}

	metrics, err := common.GetMetrics("Testnet")
	if err != nil {
		glog.Fatal("metrics: ", err)
	}

	chain, err := dbtestdata.NewFakeBlockChain(parser)
	if err != nil {
		glog.Fatal("metrics: ", err)
	}

	binding := "localhost:" + strconv.Itoa(port)

	s, err := NewPublicServer(binding, "", db, chain, nil, "", metrics, is, false)
	if err != nil {
		t.Fatal(err)
	}
	return s, binding, path
}

func closeAndDestroyPublicServer(t *testing.T, s *PublicServer, dbpath string) {

	// destroy db
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	os.RemoveAll(dbpath)
}

func Test_PublicServer_UTXO(t *testing.T) {
	s, _, dbpath := setupPublicHTTPServer(t)
	defer closeAndDestroyPublicServer(t, s, dbpath)
}
