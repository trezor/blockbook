[![Go Report Card](https://goreportcard.com/badge/trezor/blockbook)](https://goreportcard.com/report/trezor/blockbook)

# Blockbook - Satoxcoin Edition

**Modified for Satoxcoin by Satoxcoin Core Developers**

This is a modified version of the original Blockbook project, customized to support Satoxcoin (SATOX) as the default blockchain explorer. The original Blockbook project is developed by Trezor and is licensed under the GNU Affero General Public License v3.0.

**Original Project:** [Trezor Blockbook](https://github.com/trezor/blockbook)  
**Satoxcoin Implementation:** Satoxcoin Core Developers  
**License:** GNU Affero General Public License v3.0 (see COPYING file)

## Satoxcoin Features

- **Default Coin:** Satoxcoin (SATOX) is set as the default blockchain
- **SLIP44 Support:** Uses SLIP 9007 (BIP44 Coin Type: 1669) for Satoxcoin
- **Network Support:** Mainnet, testnet, and regtest configurations
- **CoinGecko Integration:** Real-time price data for SATOX
- **Multi-Network:** Supports both mainnet and testnet environments

## System Requirements

### Supported Operating Systems
- **Ubuntu 22.04 LTS** (jammy) - ✅ Fully Supported
- **Ubuntu 20.04 LTS** (focal) - ✅ Compatible
- **Debian 11+** - ✅ Compatible
- **Other Linux distributions** - ✅ Compatible with Go 1.23+

### Hardware Requirements
- **RAM:** Minimum 4GB, Recommended 8GB+ for mainnet
- **Storage:** Minimum 50GB, Recommended 100GB+ for full blockchain
- **CPU:** Multi-core processor recommended
- **Network:** Stable internet connection for blockchain sync

### Dependencies
- **Go:** 1.23.0 or higher
- **RocksDB:** Included via `github.com/linxGnu/grocksdb v1.9.8`
- **ZeroMQ:** `github.com/pebbe/zmq4 v1.2.1`
- **Ethereum Libraries:** `github.com/ethereum/go-ethereum v1.15.5`
- **Bitcoin Libraries:** `github.com/martinboehm/btcd` and `github.com/martinboehm/btcutil`

## Installation

### Quick Start (Ubuntu 22.04)
```bash
# Install Go 1.23+
sudo apt update
sudo apt install golang-go

# Clone and build
git clone <repository-url>
cd satoxcoin-blockbook
go build -o blockbook .

# Run with default Satoxcoin configuration
./blockbook
```

## Usage

```bash
# Run with default Satoxcoin mainnet configuration (port 6110)
./blockbook

# Run with testnet configuration (port 6110)
./blockbook -blockchaincfg=blockchaincfg_testnet.json

# Run with custom configuration
./blockbook -blockchaincfg=path/to/custom/config.json

# Run with low-resource configuration (port 6110)
./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50

### Satoxcoin Node Configuration

Before running the blockbook, ensure your Satoxcoin node is properly configured:

1. **Create Satoxcoin configuration file:**
   ```bash
   # Create config directory
   mkdir -p ~/.satoxcoin
   
   # Copy example configuration
   cp satoxcoin.conf.example ~/.satoxcoin/satoxcoin.conf
   
   # Edit with your credentials
   nano ~/.satoxcoin/satoxcoin.conf
   ```

2. **Update credentials in the config file:**
   ```bash
   rpcuser=your_actual_username
   rpcpassword=your_actual_password
   ```

3. **Start Satoxcoin node:**
   ```bash
   satoxcoind
   ```

4. **Verify RPC connection:**
   ```bash
   satoxcoin-cli getblockchaininfo
   ```

### Docker Support
The project uses the existing blockbook Docker infrastructure:

```bash
# Build using existing Docker infrastructure
make build

# Build optimized for VPS
make build ARGS="-ldflags='-s -w'"

# Run with low-resource configuration
./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50
```

---

# Blockbook

**Blockbook** is a back-end service for Trezor Suite. The main features of **Blockbook** are:

-   index of addresses and address balances of the connected block chain
-   fast index search
-   simple blockchain explorer
-   websocket, API and legacy Bitcore Insight compatible socket.io interfaces
-   support of multiple coins (Bitcoin and Ethereum type) with easy extensibility to other coins
-   scripts for easy creation of debian packages for backend and blockbook

## Build and installation instructions

Officially supported platform is **Debian Linux** and **AMD64** architecture.

Memory and disk requirements for initial synchronization of **Bitcoin mainnet** are around 32 GB RAM and over 180 GB of disk space. After initial synchronization, fully synchronized instance uses about 10 GB RAM.
Other coins should have lower requirements, depending on the size of their block chain. Note that fast SSD disks are highly
recommended.

User installation guide is [here](<https://wiki.trezor.io/User_manual:Running_a_local_instance_of_Trezor_Wallet_backend_(Blockbook)>).

Developer build guide is [here](/docs/build.md).

Contribution guide is [here](CONTRIBUTING.md).

## Implemented coins

Blockbook currently supports over 30 coins. The Trezor team implemented

-   Bitcoin, Bitcoin Cash, Zcash, Dash, Litecoin, Bitcoin Gold, Ethereum, Ethereum Classic, Dogecoin, Namecoin, Vertcoin, DigiByte, Liquid

the rest of coins were implemented by the community, including:

-   Satoxcoin (SATOX) - Custom implementation by Satoxcoin Core Developers

Testnets for some coins are also supported, for example:

-   Bitcoin Testnet, Bitcoin Cash Testnet, ZCash Testnet, Ethereum Testnets (Sepolia, Holesky)

List of all implemented coins is in [the registry of ports](/docs/ports.md).

## Common issues when running Blockbook or implementing additional coins

#### Out of memory when doing initial synchronization

How to reduce memory footprint of the initial sync:

-   disable rocksdb cache by parameter `-dbcache=0`, the default size is 500MB
-   run blockbook with parameter `-workers=1`. This disables bulk import mode, which caches a lot of data in memory (not in rocksdb cache). It will run about twice as slowly but especially for smaller blockchains it is no problem at all.

Please add your experience to this [issue](https://github.com/trezor/blockbook/issues/43).

#### Error `internalState: database is in inconsistent state and cannot be used`

Blockbook was killed during the initial import, most commonly by OOM killer.
By default, Blockbook performs the initial import in bulk import mode, which for performance reasons does not store all data immediately to the database. If Blockbook is killed during this phase, the database is left in an inconsistent state.

See above how to reduce the memory footprint, delete the database files and run the import again.

Check [this](https://github.com/trezor/blockbook/issues/89) or [this](https://github.com/trezor/blockbook/issues/147) issue for more info.

#### Running on Ubuntu

[This issue](https://github.com/trezor/blockbook/issues/45) discusses how to run Blockbook on Ubuntu. If you have some additional experience with Blockbook on Ubuntu, please add it to [this issue](https://github.com/trezor/blockbook/issues/45).

#### My coin implementation is reporting parse errors when importing blockchain

Your coin's block/transaction data may not be compatible with `BitcoinParser` `ParseBlock`/`ParseTx`, which is used by default. In that case, implement your coin in a similar way we used in case of [zcash](https://github.com/trezor/blockbook/tree/master/bchain/coins/zec) and some other coins. The principle is not to parse the block/transaction data in Blockbook but instead to get parsed transactions as json from the backend.

## Data storage in RocksDB

Blockbook stores data the key-value store RocksDB. Database format is described [here](/docs/rocksdb.md).

## API

Blockbook API is described [here](/docs/api.md).

## Environment variables

List of environment variables that affect Blockbook's behavior is [here](/docs/env.md).
