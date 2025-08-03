# Changelog - Satoxcoin Blockbook

All notable changes to the Satoxcoin Blockbook project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Initial Satoxcoin support implementation
- SLIP 9007 (BIP44 Coin Type: 1669) integration
- Satoxcoin as default blockchain explorer
- Mainnet, testnet, and regtest network support
- CoinGecko integration for real-time SATOX price data
- **VPS Optimization Features:**
  - Low-resource configuration files
  - Docker Compose setup with resource limits
  - VPS optimization script
  - Memory and CPU usage optimizations
  - Reduced worker threads and chunk sizes
  - Optimized database cache settings
- Custom network parameters for Satoxcoin:
  - Mainnet Magic Bytes: `0x63656556` (S A T T)
  - Testnet Magic Bytes: `0x63656556` (S A T T)
  - Regtest Magic Bytes: `0x444f5752` (D R O W)
  - Mainnet Address Prefix: `S` (Base58 prefix 99)
  - Testnet Address Prefix: `S` (Base58 prefix 99)
  - RPC Ports: 7777 (Mainnet), 19766 (Testnet)
  - P2P Ports: 60777 (Mainnet), 7060 (Testnet)

### Changed
- Modified `blockbook.go` to use Satoxcoin as default configuration
- Updated blockchain registration to include Satoxcoin implementations
- Added Satoxcoin-specific configuration files

### Technical Details
- **Go Version:** 1.23.0 (compatible with Ubuntu 22.04 LTS)
- **Dependencies:** Updated to latest stable versions
- **Build System:** Compatible with existing blockbook build infrastructure
- **License:** GNU Affero General Public License v3.0

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

### Docker Support
The project uses the existing blockbook Docker infrastructure:

```bash
# Build using existing Docker infrastructure
make build

# Build optimized for VPS
make build ARGS="-ldflags='-s -w'"

# Run with low-resource configuration (port 6110)
./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50

### Satoxcoin Node Configuration
- Added `satoxcoin.conf.example` with proper RPC configuration
- Updated all config files to use correct RPC credentials
- Added documentation for Satoxcoin node setup

## Configuration

### Default Configuration Files
- `blockchaincfg.json` - Mainnet configuration
- `blockchaincfg_testnet.json` - Testnet configuration
- `configs/coins/satoxcoin.json` - Mainnet package configuration
- `configs/coins/satoxcoin_testnet.json` - Testnet package configuration

### Environment Variables
- `BLOCKBOOK_DATADIR` - Database directory (default: `./data`)
- `BLOCKBOOK_DBCACHE` - RocksDB cache size (default: 1GB)
- `BLOCKBOOK_INTERNAL` - Internal API binding (default: `:9030`)
- `BLOCKBOOK_PUBLIC` - Public API binding (default: `:9130`)

## API Endpoints

### Mainnet
- **Internal API:** `http://localhost:9030`
- **Public API:** `http://localhost:9130`
- **WebSocket:** `ws://localhost:9130/websocket`

### Testnet
- **Internal API:** `http://localhost:9197`
- **Public API:** `http://localhost:9297`
- **WebSocket:** `ws://localhost:9297/websocket`

## Development

### Building from Source
```bash
# Clone repository
git clone <repository-url>
cd satoxcoin-blockbook

# Install dependencies
go mod download

# Build binary
go build -o blockbook .

# Run tests
go test ./bchain/coins/satoxcoin
```

### Testing
```bash
# Run all tests
go test ./...

# Run Satoxcoin-specific tests
go test ./bchain/coins/satoxcoin

# Run integration tests
go test ./tests
```

## Contributing

This project follows the same contribution guidelines as the original Trezor Blockbook project. Please refer to `CONTRIBUTING.md` for detailed information.

## License

This project is licensed under the GNU Affero General Public License v3.0. See the `COPYING` file for details.

## Acknowledgments

- **Original Project:** [Trezor Blockbook](https://github.com/trezor/blockbook)
- **Satoxcoin Implementation:** Satoxcoin Core Developers
- **License:** GNU Affero General Public License v3.0 