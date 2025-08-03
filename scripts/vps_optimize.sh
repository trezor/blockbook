#!/bin/bash

# Satoxcoin Blockbook VPS Optimization Script
# This script optimizes the system for running blockbook on low-spec VPS servers

echo "🚀 Optimizing VPS for Satoxcoin Blockbook..."

# Check system resources
echo "📊 System Information:"
echo "CPU Cores: $(nproc)"
echo "Total RAM: $(free -h | grep Mem | awk '{print $2}')"
echo "Available RAM: $(free -h | grep Mem | awk '{print $7}')"
echo "Disk Space: $(df -h / | tail -1 | awk '{print $4}') available"

# Create swap file if RAM is less than 4GB
TOTAL_RAM_KB=$(grep MemTotal /proc/meminfo | awk '{print $2}')
TOTAL_RAM_GB=$((TOTAL_RAM_KB / 1024 / 1024))

if [ $TOTAL_RAM_GB -lt 4 ]; then
    echo "⚠️  Low RAM detected (${TOTAL_RAM_GB}GB). Creating swap file..."
    sudo fallocate -l 2G /swapfile
    sudo chmod 600 /swapfile
    sudo mkswap /swapfile
    sudo swapon /swapfile
    echo '/swapfile none swap sw 0 0' | sudo tee -a /etc/fstab
    echo "✅ Swap file created (2GB)"
else
    echo "✅ Sufficient RAM detected (${TOTAL_RAM_GB}GB)"
fi

# Optimize system settings
echo "🔧 Optimizing system settings..."

# Increase file descriptor limits
echo "* soft nofile 65536" | sudo tee -a /etc/security/limits.conf
echo "* hard nofile 65536" | sudo tee -a /etc/security/limits.conf

# Optimize kernel parameters
echo "vm.swappiness=10" | sudo tee -a /etc/sysctl.conf
echo "vm.dirty_ratio=15" | sudo tee -a /etc/sysctl.conf
echo "vm.dirty_background_ratio=5" | sudo tee -a /etc/sysctl.conf

# Apply sysctl changes
sudo sysctl -p

# Create optimized blockbook startup script
cat > start_blockbook_optimized.sh << 'EOF'
#!/bin/bash

# Satoxcoin Blockbook - Optimized for VPS
# Usage: ./start_blockbook_optimized.sh

echo "🚀 Starting Satoxcoin Blockbook (Optimized for VPS)..."

# Set environment variables for low resource usage
export GOMAXPROCS=1
export GOGC=50
export GODEBUG=gctrace=1

# Create data directory if it doesn't exist
mkdir -p ./data

# Start blockbook with optimized parameters
./blockbook \
    -blockchaincfg=blockchaincfg_low_resource.json \
    -datadir=./data \
    -dbcache=67108864 \
    -workers=1 \
    -chunk=50 \
    -internal=:6111 \
    -public=:6110 \
    -sync \
    -extendedindex \
    -enablesubnewtx \
    -logtostderr \
    -v=1

echo "✅ Blockbook stopped"
EOF

chmod +x start_blockbook_optimized.sh

# Create systemd service for auto-restart
cat > satoxcoin-blockbook.service << EOF
[Unit]
Description=Satoxcoin Blockbook
After=network.target

[Service]
Type=simple
User=$USER
WorkingDirectory=$(pwd)
ExecStart=$(pwd)/start_blockbook_optimized.sh
Restart=always
RestartSec=10
StandardOutput=journal
StandardError=journal

# Resource limits
LimitNOFILE=65536
MemoryMax=2G
CPUQuota=50%

[Install]
WantedBy=multi-user.target
EOF

echo "📋 Installation Instructions:"
echo ""
echo "1. Install Go 1.23+:"
echo "   sudo apt update && sudo apt install golang-go"
echo ""
echo "2. Build blockbook (using existing Docker infrastructure):"
echo "   make build"
echo "   # OR for VPS optimization:"
echo "   make build ARGS=\"-ldflags='-s -w'\""
echo ""
echo "3. Run with optimized settings:"
echo "   ./start_blockbook_optimized.sh"
echo ""
echo "4. (Optional) Install as systemd service:"
echo "   sudo cp satoxcoin-blockbook.service /etc/systemd/system/"
echo "   sudo systemctl daemon-reload"
echo "   sudo systemctl enable satoxcoin-blockbook"
echo "   sudo systemctl start satoxcoin-blockbook"
echo ""
echo "✅ VPS optimization complete!"
echo ""
echo "📊 Monitor resources:"
echo "   htop"
echo "   df -h"
echo "   free -h" 