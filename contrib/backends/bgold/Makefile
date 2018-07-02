BGOLD_VERSION := 0.15.1
BUILD_VERSION := 

all:
	wget https://github.com/BTCGPU/BTCGPU/releases/download/v${BGOLD_VERSION}${BUILD_VERSION}/bitcoin-gold-${BGOLD_VERSION}-x86_64-linux-gnu.tar.gz
	tar -xf bitcoin-gold-${BGOLD_VERSION}-x86_64-linux-gnu.tar.gz
	mv bitcoin-gold-${BGOLD_VERSION} bgold
	rm bgold/bin/bitcoin-qt
	rm bgold/bin/test_bitcoin

clean:
	rm -rf bgold
	rm -f bitcoin-gold-${BGOLD_VERSION}-x86_64-linux-gnu.tar.gz*
