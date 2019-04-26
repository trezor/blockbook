[![Go Report Card](https://goreportcard.com/badge/trezor/blockbook)](https://goreportcard.com/report/trezor/blockbook)

# Blockbook

> **WARNING: Blockbook is currently in the state of heavy development. We may implement at any time backwards incompatible changes that require full reindexation of the database. Also, do not expect this documentation to be always up to date.**

**Blockbook** is back-end service for Trezor wallet. Main features of **Blockbook** are:

- index of addresses and address balances of the connected block chain
- fast searches in the indexes
- simple blockchain explorer
- websocket, API and legacy Bitcore Insight compatible socket.io interfaces
- support of multiple coins (Bitcoin and Ethereum type), with easy extensibility for other coins
- scripts for easy creation of debian packages for backend and blockbook

## Build and installation instructions

Officially supported platform is **Debian Linux** and **AMD64** architecture.

Memory and disk requirements for initial synchronization of **Bitcoin mainnet** are around 32 GB RAM and over 160 GB of disk space. After initial synchronization, fully synchronized instance uses about 10 GB RAM.
Other coins should have lower requirements, depending on the size of their block chain. Note that fast SSD disks are highly
recommended.

User installation guide is [here](https://wiki.trezor.io/User_manual:Running_a_local_instance_of_Trezor_Wallet_backend_(Blockbook)).

Developer build guide is [here](/docs/build.md).

Contribution guide is [here](CONTRIBUTING.md).

## Implemented coins

Blockbook currently supports over 20 coins, among them:
- Bitcoin, Litecoin, Bitcoin Cash, Bgold, ZCash, Dash, Ethereum, Ethereum Classic

Testnets for some coins are also supported, for example:
- Bitcoin Testnet, Bitcoin Cash Testnet, ZCash Testnet, Ethereum Testnet Ropsten

List of all implemented coins is in [the registry of ports](/docs/ports.md).

## Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Database format is described [here](/docs/rocksdb.md).

## API

Blockbook API is described [here](/docs/api.md).
