#!/usr/bin/env sh

echo "Installing go..."
# Pick one of the following methods to install go
sudo apt install golang-go
# cd /tmp
# wget https://golang.org/dl/go1.21.4.linux-amd64.tar.gz
# tar xf go1.21.4.linux-amd64.tar.gz
# sudo mv go /opt/go
# sudo ln -s /opt/go/bin/go /usr/bin/go
mkdir -p $HOME/go
export GOPATH=$HOME/go
export PATH=$PATH:$GOPATH/bin
echo "Consider installing VSCode Go extension"

echo "Installing zeromq..."
cd /tmp
git clone https://github.com/zeromq/libzmq
cd libzmq
./autogen.sh
./configure
make
sudo make install

echo "Installing rocksdb..."
sudo apt-get update
sudo apt-get install -y build-essential git wget pkg-config libzmq3-dev libgflags-dev libsnappy-dev zlib1g-dev libzstd-dev libbz2-dev liblz4-dev
cd $HOME
git clone https://github.com/facebook/rocksdb.git
cd rocksdb
git checkout v7.5.3
CFLAGS=-fPIC CXXFLAGS=-fPIC make release
export CGO_CFLAGS="-I$HOME/rocksdb/include"
export CGO_LDFLAGS="-L$HOME/rocksdb -lrocksdb -lstdc++ -lm -lz -ldl -lbz2 -lsnappy -llz4 -lzstd"
