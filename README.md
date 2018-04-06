# blockbook

## Install using Docker:

```
git clone https://github.com/jpochyla/blockbook.git
cd blockbook/docker
./build.sh
```

## Install

Setup go environment (Debian 9):

```
sudo apt-get update && apt-get install -y \
    build-essential git wget pkg-config lxc-dev libzmq3-dev libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev
cd /opt
wget https://storage.googleapis.com/golang/go1.9.2.linux-amd64.tar.gz && tar xf go1.9.2.linux-amd64.tar.gz
sudo ln -s /opt/go/bin/go /usr/bin/go
go help gopath
```

Install RocksDB: https://github.com/facebook/rocksdb/blob/master/INSTALL.md
and compile the static_lib and tools

```
git clone https://github.com/facebook/rocksdb.git
cd rocksdb
make release
```

Setup variables for gorocksdb: https://github.com/tecbot/gorocksdb

```
export CGO_CFLAGS="-I/path/to/rocksdb/include"
export CGO_LDFLAGS="-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4"
```

Install ZeroMQ: https://github.com/zeromq/libzmq

Install go-dep tool:
```
RUN go get github.com/golang/dep/cmd/dep
```

Get blockbook sources, install dependencies, build:

```
cd $GOPATH/src
git clone https://github.com/jpochyla/blockbook.git
cd blockbook
dep ensure
go build
```

## Usage

```
./blockbook --help
```

## Example command
To run blockbook with fast synchronization, connection to ZeroMQ and providing https and socket.io interface, with database in local directory *data* and connected to local bitcoind at http://localhost:8332 with user rpc/rpc:
```
./blockbook -sync -parse -httpserver=127.0.0.1:8333 -socketio=127.0.01:8334 -certfile=server/testcert -zeromq=tcp://127.0.0.1:28332
```
Blockbook logs only to stderr, logging to files is disabled. Verbosity of logs can be tuned by command line parameters *-v* and *-vmodule*, details at https://godoc.org/github.com/golang/glog


# Supported coins

- [BTC](bchain/coins/btc/btc.md)
- [BTC Testnet](bchain/coins/btc/btctestnet.md)
- [ZCash](bchain/coins/zec/zec.md)
- [Ethereum Ropsten Testnet](bchain/coins/eth/ethropsten.md)

# Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Data are stored in binary form to save space.
The data are separated to different column families:

- **default**

  at the moment not used, will store statistical data etc.

- **height** - maps *block height* to *block hash*

  *Block heigh* stored as array of 4 bytes (big endian uint32)  
  *Block hash* stored as array of 32 bytes

  Example - the first four blocks (all data hex encoded)
```
0x00000000 : 0x000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943
0x00000001 : 0x00000000b873e79784647a6c82962c70d228557d24a747ea4d1b8bbe878e1206
0x00000002 : 0x000000006c02c8ea6e4ff69651f7fcde348fb9d557a06e6957b65552002a7820
0x00000003 : 0x000000008b896e272758da5297bcd98fdc6d97c9b765ecec401e286dc1fdbe10
```

- **outputs** -  maps *output script+block height* to *array of outpoints*

  *Output script (ScriptPubKey)+block height* stored as variable length array of bytes for output script + 4 bytes (big endian uint32) block height  
  *array of outpoints* stored as array of 32 bytes for transaction id + variable length outpoint index for each outpoint

  Example - (all data hex encoded)
```
0x001400efeb484a24a1c1240eafacef8566e734da429c000e2df6 : 0x1697966cbd76c75eb9fc736dfa3ba0bc045999bab1e8b10082bc0ba546b0178302
0xa9143e3d6abe282d92a28cb791697ba001d733cefdc7870012c4b1 : 0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d301
```

- **inputs** - maps *transaction outpoint* to *input transaction* that spends it

  *Transaction outpoint* stored as array of 32 bytes for transaction id + variable length outpoint index  
  *Input transaction* stored as array of 32 bytes for transaction id + variable length input index

  Example - (all data hex encoded)
```
0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d300 : 0x0a7aa90ea0269c79f844c516805e4cac594adb8830e56fca894b66aab19136a428
0x7246e79f97b5f82e7f51e291d533964028ec90be0634af8a8ef7d5a903c7f6d301 : 0x4303a9fcfe6026b4d33ba488df6443c9a99bca7b7fcb7c6f6cd65cea24a749b700
```

## Todo

- find memory leak in initial import
- support all coins from https://github.com/trezor/trezor-common/blob/master/coins.json
- full ethereum support
- cleanup of the socket.io - do not send unnecessary data
- handle different versions of Bitcoin Core
- log live traffic from production bitcore server and replay it in blockbook
- protobuf websocket interface
- xpub index
- tests
- fix program dependencies to concrete versions
- protect blockbook against illicit usage
- ~~zcash support~~
- ~~basic ethereum support~~
- ~~disconnect blocks - use block data if available to avoid full scan~~
- ~~compute statistics of data, txcache, usage, etc.~~
- ~~disconnect blocks - remove disconnected cached transactions~~
- ~~implement getmempoolentry~~
- ~~support altcoins, abstraction of blockchain server/service~~
- ~~cache transactions in RocksDB~~
- ~~parallel sync - rewrite - it is not possible to gracefully stop it now, can leave holes in the block~~
- ~~mempool - return also input transactions~~
- ~~blockchain - return inputs from mempool~~
- ~~do not return duplicate txids~~
- ~~legacy socket.io JSON interface~~
- ~~disconnect blocks - optimize - full range scan is too slow and takes too much disk space (creates snapshot of the whole outputs), split to multiple iterators~~
- ~~parallel sync - let rocksdb to compact itself from time to time, otherwise it consumes too much disk space~~
