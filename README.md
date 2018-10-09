# Blockbook

> **WARNING: Blockbook is currently in the state of heavy development. We may implement at any time backwards incompatible changes, that require full reindexation of the database. Also, do not expect this documentation to be always up to date.**

**Blockbook** is back-end service for Trezor wallet. Main features of **Blockbook** are:

- create missing indexes in the blockchain - index of addresses and address balances
- allow fast searches in the index of addresses
- implement parts Insight socket.io interface as required by Trezor wallet
- support of multiple coins
- simple blockchain explorer for implemented coins
- scripts for easy creation of debian packages for backend and blockbook

## Build and installation instructions

Officially supported platform is **Debian Linux** and **AMD64** architecture. 

Memory and disk requirements for initial synchronization of **Bitcoin mainnet** are around 32 GB RAM and over 150 GB of disk size. After initial synchronization, fully synchronized instance takes around 10 GB RAM.
Other coins should have lower requirements depending on size of their block chain. Note that fast SSD disks are highly
recommended.

User installation guide is [here](https://wiki.trezor.io/User_manual:Running_a_local_instance_of_Trezor_Wallet_backend_(Blockbook)).

Developer build guide is [here](/docs/build.md).

Contribution guide is [here](CONTRIBUTING.md).

# Implemented coins

The most significant coins implemented by Blockbook are:
- Bitcoin, Bcash, Bgold, ZCash, Dash, Litecoin

Incomplete, experimental support is for:
- Ethereum, Ethereum Classic

Testnets for some coins are also supported, for example:
- Bitcoin Testnet, Bcash Testnet, ZCash Testnet, Ethereum Testnet Ropsten

List of all implemented coins is in [the registry of ports](/docs/ports.md).

# Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Database format is described [here](/docs/rocksdb.md).

