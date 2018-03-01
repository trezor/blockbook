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

Install gorocksdb: https://github.com/tecbot/gorocksdb

```
CGO_CFLAGS="-I/path/to/rocksdb/include" \
CGO_LDFLAGS="-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4" \
  go get github.com/tecbot/gorocksdb
```

Install ZeroMQ: https://github.com/zeromq/libzmq

Install Go interface to ZeroMQ:
```
go get github.com/pebbe/zmq4
```

Install additional go libraries:
```
go get github.com/golang/glog
go get github.com/martinboehm/golang-socketio
go get github.com/btcsuite/btcd
go get github.com/gorilla/handlers
go get github.com/bsm/go-vlq
go get github.com/gorilla/handlers
go get github.com/gorilla/mux
go get github.com/pebbe/zmq4
go get github.com/pkg/profile
go get github.com/juju/errors
```

Install blockbook:

```
cd $GOPATH/src
git clone https://github.com/jpochyla/blockbook.git
cd blockbook
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

## Setup on the blockbook-dev server (including Bitcoin Core)
Get Bitcoin Core
```
wget https://bitcoin.org/bin/bitcoin-core-0.15.1/bitcoin-0.15.1-x86_64-linux-gnu.tar.gz
tar -xf bitcoin-0.15.1-x86_64-linux-gnu.tar.gz 
```

### TESTNET ###
Data are to be stored in */data/testnet*, in folders */data/testnet/bitcoin* for Bitcoin Core data, */data/testnet/blockbook* for Blockbook data.

Create configuration file */data/testnet/bitcoin/bitcoin.conf* with content
```
testnet=1
daemon=1
server=1
rpcuser=rpc
rpcpassword=rpc
rpcport=18332
txindex=1
```
Create script that starts the bitcoind daemon *run-testnet-bitcoind.sh*
```
#!/bin/bash

bitcoin-0.15.1/bin/bitcoind -datadir=/data/testnet/bitcoin -zmqpubhashtx=tcp://127.0.0.1:18334 -zmqpubhashblock=tcp://127.0.0.1:18334 -zmqpubrawblock=tcp://127.0.0.1:18334 -zmqpubrawtx=tcp://127.0.0.1:18334
```
Run the *run-testnet-bitcoind.sh* to get initial import of data.

Create script that runs blockbook *run-testnet-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -path=/data/testnet/blockbook/db -sync -parse -rpcurl=http://127.0.0.1:18332 -httpserver=:18335 -socketio=:18336 -certfile=server/testcert -zeromq=tcp://127.0.0.1:18334 -explorer=https://testnet-bitcore1.trezor.io $1
```
To run blockbook with logging to file (run with nohup or daemonize or using screen)
```
./run-testnet-blockbook.sh 2>/data/testnet/blockbook/blockbook.log
```

### BTC ###
Data are to be stored in */data/btc*, in folders */data/btc/bitcoin* for Bitcoin Core data, */data/btc/blockbook* for Blockbook data.

Create configuration file */data/btc/bitcoin/bitcoin.conf* with content
```
daemon=1
server=1
rpcuser=rpc
rpcpassword=rpc
rpcport=8332
txindex=1
```
Create script that starts the bitcoind daemon *run-btc-bitcoind.sh*
```
#!/bin/bash

bitcoin-0.15.1/bin/bitcoind -datadir=/data/btc/bitcoin -zmqpubhashtx=tcp://127.0.0.1:8334 -zmqpubhashblock=tcp://127.0.0.1:8334 -zmqpubrawblock=tcp://127.0.0.1:8334 -zmqpubrawtx=tcp://127.0.0.1:8334
```
Run the *run-btc-bitcoind.sh* to get initial import of data.

Create script that runs blockbook *run-btc-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -path=/data/btc/blockbook/db -sync -parse -rpcurl=http://127.0.0.1:8332 -httpserver=:8335 -socketio=:8336 -certfile=server/testcert -zeromq=tcp://127.0.0.1:8334 -explorer=https://bitcore1.trezor.io/ $1
```
To run blockbook with logging to file  (run with nohup or daemonize or using screen)
```
./run-btc-blockbook.sh 2>/data/btc/blockbook/blockbook.log
```


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

- cleanup of the socket.io - do not send unnecessary data
- protobuf websocket interface
- parallel sync - rewrite - it is not possible to gracefully stop it now, can leave holes in the block
- disconnect blocks - keep map of transactions in the last 100 blocks
- handle different versions of Bitcoin Core
- limit number of transactions returned by rocksdb.GetTransactions
- xpub index
- tests
- ~~mempool - return also input transactions~~
- ~~blockchain - return inputs from mempool~~
- ~~do not return duplicate txids~~
- ~~legacy socket.io JSON interface~~
- ~~disconnect blocks - optimize - full range scan is too slow and takes too much disk space (creates snapshot of the whole outputs), split to multiple iterators~~
- ~~parallel sync - let rocksdb to compact itself from time to time, otherwise it consumes too much disk space~~
