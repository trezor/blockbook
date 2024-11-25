#!/usr/bin/env sh

set -e

echo "Going to /tmp..."
cd /tmp

TARBALL=bitcoin-28.0-x86_64-linux-gnu.tar.gz

echo "Removing previously downloaded tarball when it exists..."
rm $TARBALL || true

echo "Downloading bitcoincore 28.0 tarball..."
wget https://bitcoincore.org/bin/bitcoin-core-28.0/$TARBALL


echo "Extracing the tarball..."
tar xzf $TARBALL

echo "Navigating to the directory with bitcoind and bitcoin-cli binaries..."
cd bitcoin-28.0
cd bin

echo "Installing bitcoind - please supply sudo password"
sudo install -m 0755 -o root -g root -t /usr/local/bin *
