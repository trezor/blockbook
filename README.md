# Blockbook

> **Blockbook is currently in the state of heavy development, do not expect this documentation to be up to date**

## Build and installation instructions

Officially supported platform is **Debian Linux** and **AMD64** architecture. Memory requirements for initial synchronization
of **Bitcoin mainnet** are around 32 GB RAM and 100 GB of disk size. Fully synchronized instance takes around 10 GB RAM.
Other coins should have lower requirements depending on size of their block chain. Note that fast SSD disks are highly
recommended.

User installation guide is [here](https://wiki.trezor.io/User_manual:Running_a_local_instance_of_Trezor_Wallet_backend_(Blockbook)).

Developer build guide is [here](/docs/build.md).

# Implemented coins

The most significant coins implemented by Blockbook are:

- Bitcoin
- Bitcoin Testnet
- Bcash
- Bcash Testnet
- Bgold
- ZCash
- ZCash Testnet
- Dash
- Dash Testnet
- Litecoin
- Litecoin Testnet
- Ethereum
- Ethereum Testnet Ropsten

They are also supported by Trezor wallet. List of all coins is [here](/docs/ports.md).

# Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Database format is described [here](/docs/rocksdb.md).

## Registry of ports

Reserved ports are described [here](/docs/ports.md)

## Todo

- add db data version (column data version) checking to db to avoid data corruption
- improve txcache (time of storage, number/size of cached txs, purge cache)
- create/integrate blockchain explorer
- support all coins from https://github.com/trezor/trezor-common/tree/master/defs/coins
- full ethereum support (tokens, balance)
- protobuf websocket interface instead of socket.io
- xpub index
- tests
- fix program dependencies to concrete versions
- protect socket.io interface against illicit usage
- ~~update documentation~~
- ~~collect blockbook db stats (number of items in indexes, etc)~~
- ~~optimize mempool (use non verbose get transaction, possibly parallelize)~~
- ~~update used paths and users according to specification by system admin~~
- ~~cleanup of the socket.io - do not send unnecessary data~~
- ~~handle different versions of Bitcoin Core~~
- ~~log live traffic from production bitcore server and replay it in blockbook~~
- ~~find memory leak in initial import - disappeared with index v2~~
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
