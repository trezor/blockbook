module github.com/trezor/blockbook

go 1.17

require (
	github.com/Groestlcoin/go-groestl-hash v0.0.0-20181012171753-790653ac190c // indirect
	github.com/bsm/go-vlq v0.0.0-20150828105119-ec6e8d4f5f4e
	github.com/dchest/blake256 v1.0.0 // indirect
	github.com/deckarep/golang-set v1.8.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.2
	github.com/decred/dcrd/chaincfg/v3 v3.0.0
	github.com/decred/dcrd/dcrec v1.0.0
	github.com/decred/dcrd/dcrjson/v3 v3.0.1
	github.com/decred/dcrd/dcrutil/v3 v3.0.0
	github.com/decred/dcrd/hdkeychain/v3 v3.0.0
	github.com/decred/dcrd/txscript/v3 v3.0.0
	github.com/ethereum/go-ethereum v1.10.23
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.5.2
	github.com/gorilla/websocket v1.4.2
	github.com/juju/errors v0.0.0-20170703010042-c7d06af17c68
	github.com/juju/loggo v0.0.0-20190526231331-6e530bcce5d8 // indirect
	github.com/juju/testing v0.0.0-20191001232224-ce9dec17d28b // indirect
	github.com/linxGnu/grocksdb v1.7.7
	github.com/martinboehm/bchutil v0.0.0-20190104112650-6373f11b6efe
	github.com/martinboehm/btcd v0.0.0-20221101112928-408689e15809
	github.com/martinboehm/btcutil v0.0.0-20211010173611-6ef1889c1819
	github.com/martinboehm/golang-socketio v0.0.0-20180414165752-f60b0a8befde
	github.com/mr-tron/base58 v1.2.0 // indirect
	github.com/pebbe/zmq4 v1.2.1
	github.com/pirk/ecashaddr-converter v0.0.0-20220121162910-c6cb45163b29
	github.com/pirk/ecashutil v0.0.0-20220124103933-d37f548d249e
	github.com/prometheus/client_golang v1.8.0
	github.com/schancel/cashaddr-converter v0.0.0-20181111022653-4769e7add95a
	golang.org/x/crypto v0.0.0-20210921155107-089bfa567519
	google.golang.org/protobuf v1.26.0
	gopkg.in/mgo.v2 v2.0.0-20190816093944-a6b53ec6cb22 // indirect
)

require (
	github.com/PiRK/cashaddr-converter v0.0.0-20220121162910-c6cb45163b29 // indirect
	github.com/StackExchange/wmi v0.0.0-20180116203802-5d049714c4a6 // indirect
	github.com/agl/ed25519 v0.0.0-20170116200512-5312a6153412 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/btcsuite/btcd v0.20.1-beta // indirect
	github.com/btcsuite/btcd/btcec/v2 v2.2.0 // indirect
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	github.com/cespare/xxhash/v2 v2.1.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dchest/siphash v1.2.1 // indirect
	github.com/decred/base58 v1.0.3 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect
	github.com/decred/dcrd/crypto/ripemd160 v1.0.1 // indirect
	github.com/decred/dcrd/dcrec/edwards/v2 v2.0.1 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v3 v3.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/decred/dcrd/wire v1.4.0 // indirect
	github.com/decred/slog v1.1.0 // indirect
	github.com/go-ole/go-ole v1.2.1 // indirect
	github.com/go-stack/stack v1.8.0 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.14.0 // indirect
	github.com/prometheus/procfs v0.2.0 // indirect
	github.com/shirou/gopsutil v3.21.4-0.20210419000835-c7a38de76ee5+incompatible // indirect
	github.com/stretchr/testify v1.8.0 // indirect
	github.com/tklauser/go-sysconf v0.3.5 // indirect
	github.com/tklauser/numcpus v0.2.2 // indirect
	golang.org/x/sys v0.0.0-20220520151302-bc2c85ada10a // indirect
	gopkg.in/natefinch/npipe.v2 v2.0.0-20160621034901-c1b8fa8bdcce // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// replace github.com/martinboehm/btcutil => ../btcutil

// replace github.com/martinboehm/btcd => ../btcd
