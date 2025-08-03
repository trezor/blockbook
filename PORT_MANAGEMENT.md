# Satoverse.io Port Management

## Port Range Allocation: 6000-6999 (Satoverse.io Ecosystem Services)

### Purpose
This document defines the port allocation strategy for the Satoverse.io ecosystem to avoid conflicts with existing Docker services and maintain organization. This includes P2E Platform, Explorers, DNS Seeds, RPC Services, APIs, and Web Services.

### Current Docker Services (Avoid These Ports)
- **3000**: Grafana (satox-grafana)
- **8080**: cAdvisor (satox-cadvisor)
- **9090**: Prometheus (satox-prometheus)
- **3100**: Loki (satox-loki)
- **9093**: Alertmanager (satox-alertmanager)
- **9100**: Node Exporter (satox-node-exporter)
- **7777**: SatoxCoin RPC Port (Reserved for blockchain node communication)
- **60777**: SatoxCoin P2P Port (Reserved for peer-to-peer network communication)

### Satoverse.io Ecosystem Port Allocation

#### P2E Platform Services (6000-6099)
- **6000**: Satoverse.io P2E Platform API
- **6001**: Satoverse.io P2E Platform Health
- **6002**: Satoverse.io P2E Platform Metrics
- **6003**: Satoverse.io P2E Platform Documentation
- **6004**: Satoverse.io P2E Platform Testing
- **6005**: Satoverse.io P2E Platform Examples
- **6006**: Satoverse.io P2E Platform Admin
- **6007**: Satoverse.io P2E Platform WebSocket
- **6008**: Satoverse.io P2E Platform gRPC
- **6009**: Satoverse.io P2E Platform GraphQL
- **6010**: Satoverse.io P2E Platform Frontend
- **6011**: Satoverse.io P2E Platform Backend
- **6012**: Satoverse.io P2E Platform Database
- **6013**: Satoverse.io P2E Platform Redis
- **6014**: Satoverse.io P2E Platform MongoDB
- **6015**: Satoverse.io P2E Platform Blockchain

#### Explorer Services (6100-6199)

**Main Explorer (6100-6109):**
- **6100**: Satoverse.io Explorer API
- **6101**: Satoverse.io Explorer Health
- **6102**: Satoverse.io Explorer Metrics
- **6103**: Satoverse.io Explorer Documentation
- **6104**: Satoverse.io Explorer Testing
- **6105**: Satoverse.io Explorer Examples
- **6106**: Satoverse.io Explorer Admin
- **6107**: Satoverse.io Explorer WebSocket
- **6108**: Satoverse.io Explorer gRPC
- **6109**: Satoverse.io Explorer GraphQL

**Satoxcoin Blockbook (6110-6119):**
- **6110**: Satoxcoin Blockbook API (Public Interface)
- **6111**: Satoxcoin Blockbook Health (Internal Interface)
- **6112**: Satoxcoin Blockbook Metrics
- **6113**: Satoxcoin Blockbook Documentation
- **6114**: Satoxcoin Blockbook Testing
- **6115**: Satoxcoin Blockbook Examples
- **6116**: Satoxcoin Blockbook Admin
- **6117**: Satoxcoin Blockbook WebSocket
- **6118**: Satoxcoin Blockbook gRPC
- **6119**: Satoxcoin Blockbook GraphQL

**Satoxcoin Equidius (6120-6129):**
- **6120**: Satoxcoin Equidius API
- **6121**: Satoxcoin Equidius Health
- **6122**: Satoxcoin Equidius Metrics
- **6123**: Satoxcoin Equidius Documentation
- **6124**: Satoxcoin Equidius Testing
- **6125**: Satoxcoin Equidius Examples
- **6126**: Satoxcoin Equidius Admin
- **6127**: Satoxcoin Equidius WebSocket
- **6128**: Satoxcoin Equidius gRPC
- **6129**: Satoxcoin Equidius GraphQL

**Future Explorer Ports (6130-6199):**
- **6130**: Future Explorer 4 API
- **6131**: Future Explorer 4 Health
- **6132**: Future Explorer 4 Metrics
- **6133**: Future Explorer 4 Frontend
- **6134**: Future Explorer 5 API
- **6135**: Future Explorer 5 Health
- **6136**: Future Explorer 5 Metrics
- **6137**: Future Explorer 5 Frontend

#### DNS Seed Services (6200-6299)
- **6200**: Satoverse.io DNS Seed API
- **6201**: Satoverse.io DNS Seed Health
- **6202**: Satoverse.io DNS Seed Metrics
- **6203**: Satoverse.io DNS Seed Documentation
- **6204**: Satoverse.io DNS Seed Testing
- **6205**: Satoverse.io DNS Seed Examples
- **6206**: Satoverse.io DNS Seed Admin
- **6207**: Satoverse.io DNS Seed WebSocket
- **6208**: Satoverse.io DNS Seed gRPC
- **6209**: Satoverse.io DNS Seed GraphQL
- **6210**: Satoverse.io DNS Seed Frontend
- **6211**: Satoverse.io DNS Seed Backend
- **6212**: Satoverse.io DNS Seed Database
- **6213**: Satoverse.io DNS Seed Redis
- **6214**: Satoverse.io DNS Seed MongoDB
- **6215**: Satoverse.io DNS Seed Blockchain

#### RPC Services (6300-6399)
- **6300**: Satoverse.io RPC Gateway
- **6301**: Satoverse.io RPC Health
- **6302**: Satoverse.io RPC Metrics
- **6303**: Satoverse.io RPC Documentation
- **6304**: Satoverse.io RPC Testing
- **6305**: Satoverse.io RPC Examples
- **6306**: Satoverse.io RPC Admin
- **6307**: Satoverse.io RPC WebSocket
- **6308**: Satoverse.io RPC gRPC
- **6309**: Satoverse.io RPC GraphQL
- **6310**: Satoverse.io RPC Load Balancer
- **6311**: Satoverse.io RPC Backend
- **6312**: Satoverse.io RPC Database
- **6313**: Satoverse.io RPC Redis
- **6314**: Satoverse.io RPC MongoDB
- **6315**: Satoverse.io RPC Blockchain

**RPC Connection Ports (6316-6399):**
- **6316**: Explorer 1 RPC Connection
- **6317**: Explorer 2 RPC Connection
- **6318**: Explorer 3 RPC Connection
- **6319**: DNS Seed RPC Connection
- **6320**: P2E Platform RPC Connection
- **6321**: Future Service 1 RPC Connection
- **6322**: Future Service 2 RPC Connection
- **6323**: Future Service 3 RPC Connection

#### API Services (6400-6499)
- **6400**: Satoverse.io Main API
- **6401**: Satoverse.io API Health
- **6402**: Satoverse.io API Metrics
- **6403**: Satoverse.io API Documentation
- **6404**: Satoverse.io API Testing
- **6405**: Satoverse.io API Examples
- **6406**: Satoverse.io API Admin
- **6407**: Satoverse.io API WebSocket
- **6408**: Satoverse.io API gRPC
- **6409**: Satoverse.io API GraphQL
- **6410**: Satoverse.io API Gateway
- **6411**: Satoverse.io API Backend
- **6412**: Satoverse.io API Database
- **6413**: Satoverse.io API Redis
- **6414**: Satoverse.io API MongoDB
- **6415**: Satoverse.io API Blockchain

#### Web Services (6500-6599)
- **6500**: Satoverse.io Main Website
- **6501**: Satoverse.io Website Health
- **6502**: Satoverse.io Website Metrics
- **6503**: Satoverse.io Website Documentation
- **6504**: Satoverse.io Website Testing
- **6505**: Satoverse.io Website Examples
- **6506**: Satoverse.io Website Admin
- **6507**: Satoverse.io Website WebSocket
- **6508**: Satoverse.io Website gRPC
- **6509**: Satoverse.io Website GraphQL
- **6510**: Satoverse.io Website Frontend
- **6511**: Satoverse.io Website Backend
- **6512**: Satoverse.io Website Database
- **6513**: Satoverse.io Website Redis
- **6514**: Satoverse.io Website MongoDB
- **6515**: Satoverse.io Website Blockchain

#### Monitoring Services (6600-6699)
- **6600**: Satoverse.io Monitoring API
- **6601**: Satoverse.io Monitoring Health
- **6602**: Satoverse.io Monitoring Metrics
- **6603**: Satoverse.io Monitoring Documentation
- **6604**: Satoverse.io Monitoring Testing
- **6605**: Satoverse.io Monitoring Examples
- **6606**: Satoverse.io Monitoring Admin
- **6607**: Satoverse.io Monitoring WebSocket
- **6608**: Satoverse.io Monitoring gRPC
- **6609**: Satoverse.io Monitoring GraphQL
- **6610**: Satoverse.io Monitoring Dashboard
- **6611**: Satoverse.io Monitoring Backend
- **6612**: Satoverse.io Monitoring Database
- **6613**: Satoverse.io Monitoring Redis
- **6614**: Satoverse.io Monitoring MongoDB
- **6615**: Satoverse.io Monitoring Blockchain

### Usage Guidelines

#### Development
```bash
# Start P2E Platform development server
cd satoverse.io && npm run dev -- --port 6000

# Start Explorer development server
cd satoverse.io/explorer && npm run dev -- --port 6100

# Start Satoxcoin Blockbook (using existing Docker infrastructure)
cd satoxcoin-blockbook && make build
./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50

# Start DNS Seed development server
cd satoverse.io/dnsseed && npm run dev -- --port 6200

# Start RPC Gateway development server
cd satoverse.io/rpc && npm run dev -- --port 6300
```

#### Production
```bash
# Start P2E Platform production server
cd satoverse.io && npm start -- --port 6000

# Start Explorer production server
cd satoverse.io/explorer && npm start -- --port 6100

# Start Satoxcoin Blockbook production server (port 6110)
cd satoxcoin-blockbook && ./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50

# Start DNS Seed production server
cd satoverse.io/dnsseed && npm start -- --port 6200
```

#### Docker Services
```yaml
# Example docker-compose.yml for Satoverse.io ecosystem
version: '3.8'
services:
  satoverse-p2e:
    ports:
      - "6000:6000"  # P2E Platform API
      - "6001:6001"  # P2E Platform Health
      - "6002:6002"  # P2E Platform Metrics
    networks:
      - satoverse.io-network
  
  satoverse-explorer:
    ports:
      - "6100:6100"  # Explorer API (Satoxcoin Blockbook Public)
      - "6101:6101"  # Explorer Health (Satoxcoin Blockbook Internal)
      - "6102:6102"  # Explorer Metrics
    networks:
      - satoverse.io-network
  
  satoverse-dnsseed:
    ports:
      - "6200:6200"  # DNS Seed API
      - "6201:6201"  # DNS Seed Health
      - "6202:6202"  # DNS Seed Metrics
    networks:
      - satoverse.io-network
  
  satoverse-rpc:
    ports:
      - "6300:6300"  # RPC Gateway
      - "6301:6301"  # RPC Health
      - "6302:6302"  # RPC Metrics
    networks:
      - satoverse.io-network

networks:
  satoverse.io-network:
    external: true
    name: satoverse.io-network
```

### Network Configuration
- **Network Name**: `satoverse.io-network`
- **Network Type**: External bridge network
- **Service Discovery**: Container names as hostnames
- **Inter-service Communication**: HTTP/HTTPS, gRPC, WebSocket
- **Purpose**: Dedicated network for Satoverse.io ecosystem services

### RPC Connection Configuration

#### Explorer RPC Connections
```yaml
# Satoxcoin Blockbook RPC Configuration
BLOCKBOOK_RPC_URL=http://localhost:7777
BLOCKBOOK_RPC_USER=your_rpc_username
BLOCKBOOK_RPC_PASSWORD=your_rpc_password

# Satoxcoin Testnet RPC Configuration
BLOCKBOOK_TESTNET_RPC_URL=http://localhost:19766
BLOCKBOOK_TESTNET_RPC_USER=your_rpc_username
BLOCKBOOK_TESTNET_RPC_PASSWORD=your_rpc_password

# Explorer 1 RPC Configuration
EXPLORER_1_RPC_URL=http://localhost:6316
EXPLORER_1_RPC_USER=your-rpc-user
EXPLORER_1_RPC_PASSWORD=your-rpc-password

# Explorer 2 RPC Configuration
EXPLORER_2_RPC_URL=http://localhost:6317
EXPLORER_2_RPC_USER=your-rpc-user
EXPLORER_2_RPC_PASSWORD=your-rpc-password

# Explorer 3 RPC Configuration
EXPLORER_3_RPC_URL=http://localhost:6318
EXPLORER_3_RPC_USER=your-rpc-user
EXPLORER_3_RPC_PASSWORD=your-rpc-password
```

#### DNS Seed RPC Connections
```yaml
# DNS Seed RPC Configuration
DNS_SEED_RPC_URL=http://localhost:6319
DNS_SEED_RPC_USER=your-rpc-user
DNS_SEED_RPC_PASSWORD=your-rpc-password
```

#### P2E Platform RPC Connections
```yaml
# P2E Platform RPC Configuration
P2E_PLATFORM_RPC_URL=http://localhost:6320
P2E_PLATFORM_RPC_USER=your-rpc-user
P2E_PLATFORM_RPC_PASSWORD=your-rpc-password
```

### Satoverse.io Ecosystem Services
- **P2E Platform**: Play-to-earn gaming platform
- **Explorers**: Blockchain explorers for transaction and block viewing
  - **Satoxcoin Blockbook**: Primary blockchain explorer for Satoxcoin network
- **DNS Seeds**: DNS seed nodes for network discovery
- **RPC Services**: Remote procedure call services for blockchain interaction
- **API Services**: RESTful APIs for external integrations
- **Web Services**: Web interfaces and dashboards
- **Monitoring Services**: System monitoring and observability

### Satoxcoin Blockbook Integration

#### Overview
The Satoxcoin Blockbook is the primary blockchain explorer for the Satoxcoin network within the Satoverse.io ecosystem. It provides a comprehensive API for transaction and block data, address balances, and blockchain statistics.

#### Port Configuration
- **6110**: Public API interface for external applications
- **6111**: Internal health monitoring and management interface

#### Features
- **Transaction Indexing**: Fast search and retrieval of transaction data
- **Address Tracking**: Real-time balance and transaction history for addresses
- **Block Explorer**: Detailed block information and statistics
- **WebSocket Support**: Real-time updates and notifications
- **RESTful API**: Comprehensive API for external integrations
- **Low-Resource Optimization**: Optimized for VPS deployment with limited resources

#### Configuration Files
- `configs/coins/satoxcoin.json`: Mainnet configuration
- `configs/coins/satoxcoin_testnet.json`: Testnet configuration
- `configs/coins/satoxcoin_low_resource.json`: VPS-optimized configuration
- `blockchaincfg.json`: Default mainnet blockchain configuration
- `blockchaincfg_testnet.json`: Default testnet blockchain configuration
- `blockchaincfg_low_resource.json`: VPS-optimized blockchain configuration

#### Usage Examples
```bash
# Start mainnet blockbook
./blockbook

# Start testnet blockbook
./blockbook -blockchaincfg=blockchaincfg_testnet.json

# Start with VPS optimization
./blockbook -blockchaincfg=blockchaincfg_low_resource.json -dbcache=67108864 -workers=1 -chunk=50

# Build with Docker
make build ARGS="-ldflags='-s -w'"
```

#### API Endpoints
- `http://localhost:6110/api/` - Public API
- `http://localhost:6111/` - Health check and metrics
- WebSocket: `ws://localhost:6110/websocket`

#### Integration with Satoverse.io Ecosystem
- **P2E Platform**: Provides blockchain data for gaming transactions
- **RPC Services**: Connects to Satoxcoin node for data retrieval
  - **Mainnet RPC**: Port 7777 (default Satoxcoin RPC)
  - **Testnet RPC**: Port 19766 (default Satoxcoin testnet RPC)
- **Monitoring**: Health checks and metrics for system monitoring
- **API Gateway**: RESTful API for external service integration

### Blockchain Integration
- **Network**: SatoxCoin mainnet
- **RPC Endpoint**: http://satoxcoin-node:7777
- **Confirmations**: 6 blocks required
- **Gas Limit**: 21,000 (standard transaction)
- **Gas Price**: 20 Gwei (configurable)

### Monitoring and Observability
- **Metrics**: Prometheus on respective service ports
- **Health Checks**: HTTP endpoints on respective service ports
- **Logging**: Centralized via Loki
- **Tracing**: Distributed tracing enabled
- **Alerting**: Alertmanager integration

### Security Configuration
- **SSL/TLS**: Configurable (default: disabled for development)
- **Authentication**: JWT-based with quantum security
- **Authorization**: Role-based access control
- **Rate Limiting**: Enabled with configurable limits
- **CORS**: Enabled for cross-origin requests
- **API Key**: Required for external access

### Performance Configuration
- **Caching**: Redis-based with configurable TTL
- **Connection Pooling**: 10 connections (platform-optimized)
- **Load Balancing**: Nginx reverse proxy support
- **Compression**: Gzip compression enabled
- **Timeout Handling**: Configurable timeouts
- **Max Concurrent Requests**: 100 (platform-optimized)

### Benefits
1. **Conflict Avoidance**: No conflicts with existing Docker monitoring services
2. **Organization**: Clear separation of concerns by port ranges
3. **Scalability**: Room for growth and additional services
4. **Documentation**: Clear reference for developers and DevOps
5. **Consistency**: Standardized approach across all Satoverse.io ecosystem
6. **Ecosystem Integration**: Seamless integration between all Satoverse.io services
7. **Dedicated Network**: Isolated network for ecosystem services

### Current Status
- ✅ **P2E Platform API**: Running on port 6000
- ✅ **P2E Platform Health**: Running on port 6001
- ✅ **P2E Platform Metrics**: Running on port 6002
- ✅ **P2E Platform Frontend**: Running on port 6010
- ✅ **Satoxcoin Blockbook API**: Running on port 6100
- ✅ **Satoxcoin Blockbook Health**: Running on port 6101
- 🔄 **Explorer Frontend**: Planned for port 6110
- 🔄 **DNS Seed API**: Planned for port 6200
- 🔄 **RPC Gateway**: Planned for port 6300
- 🔄 **Main API**: Planned for port 6400
- 🔄 **Main Website**: Planned for port 6500
- 🔄 **Monitoring API**: Planned for port 6600

### Migration Notes
- **From Previous Ports**: Migrated from 4300-4315 to 6000-6015
- **Network**: Now using `satoverse.io-network` for ecosystem communication
- **Environment Variables**: Updated to use new port allocations
- **Health Checks**: Updated to use new health check ports
- **Documentation**: Updated to reflect new port assignments
- **Ecosystem Services**: Added dedicated ports for all ecosystem components
- **RPC Connections**: Reserved ports for inter-service RPC communication

### Notes
- Always check port availability before starting new services
- Update this document when adding new services
- Use environment variables for port configuration in production
- Consider using reverse proxy (nginx) for production deployments
- Follow the port allocation template for consistency across all services
- Ecosystem services require integration with all Satoverse.io components
- RPC connections are critical for inter-service communication
- DNS seeds require proper network configuration for peer discovery 