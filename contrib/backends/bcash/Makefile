BITCOINABC_VERSION := 0.17.1
BITCOINABC_ARCHIVE := bitcoin-abc-${BITCOINABC_VERSION}-x86_64-linux-gnu.tar.gz
BITCOINABC_SHA256 := eccf8b61ba0549f6839e586c7dc6fc4bf6d7591ac432aaea8a7df0266b113d27

all:
	wget https://download.bitcoinabc.org/${BITCOINABC_VERSION}/linux/${BITCOINABC_ARCHIVE}
	[ "$$(sha256sum ${BITCOINABC_ARCHIVE} | cut -d ' ' -f 1)" = "${BITCOINABC_SHA256}" ]
	tar -xf ${BITCOINABC_ARCHIVE}
	mv bitcoin-abc-${BITCOINABC_VERSION} bitcoin-abc
	rm bitcoin-abc/bin/bitcoin-qt
	rm bitcoin-abc/bin/test_bitcoin

clean:
	rm -rf bitcoin-abc
	rm -f ${BITCOINABC_ARCHIVE}*
