BITCOINABC_VERSION := 0.17.1

all:
	wget https://download.bitcoinabc.org/${BITCOINABC_VERSION}/linux/bitcoin-abc-${BITCOINABC_VERSION}-x86_64-linux-gnu.tar.gz
	tar -xf bitcoin-abc-${BITCOINABC_VERSION}-x86_64-linux-gnu.tar.gz
	mv bitcoin-abc-${BITCOINABC_VERSION} bitcoin-abc
	rm bitcoin-abc/bin/bitcoin-qt
	rm bitcoin-abc/bin/test_bitcoin

clean:
	rm -rf bitcoin-abc
	rm -f bitcoin-abc-${BITCOINABC_VERSION}-x86_64-linux-gnu.tar.gz*
