BITCOIN_VERSION := 0.16.1

all:
	wget https://bitcoin.org/bin/bitcoin-core-${BITCOIN_VERSION}/bitcoin-${BITCOIN_VERSION}-x86_64-linux-gnu.tar.gz
	tar -xf bitcoin-${BITCOIN_VERSION}-x86_64-linux-gnu.tar.gz
	mv bitcoin-${BITCOIN_VERSION} bitcoin
	rm bitcoin/bin/bitcoin-qt
	rm bitcoin/bin/test_bitcoin

clean:
	rm -rf bitcoin
	rm -f bitcoin-${BITCOIN_VERSION}-x86_64-linux-gnu.tar.gz*
