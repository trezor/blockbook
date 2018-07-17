BITCOIN_VERSION := 0.16.1
BITCOIN_ARCHIVE := bitcoin-${BITCOIN_VERSION}-x86_64-linux-gnu.tar.gz

all:
	wget https://bitcoin.org/bin/bitcoin-core-${BITCOIN_VERSION}/${BITCOIN_ARCHIVE}
	wget https://bitcoin.org/bin/bitcoin-core-${BITCOIN_VERSION}/SHA256SUMS.asc
	gpg --verify SHA256SUMS.asc
	sha256sum -c --ignore-missing SHA256SUMS.asc
	tar -xf ${BITCOIN_ARCHIVE}
	mv bitcoin-${BITCOIN_VERSION} bitcoin
	rm bitcoin/bin/bitcoin-qt
	rm bitcoin/bin/test_bitcoin

clean:
	rm -rf bitcoin
	rm -f ${BITCOIN_ARCHIVE}
	rm -f SHA256SUMS.asc
