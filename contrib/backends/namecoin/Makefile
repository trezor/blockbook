NAMECOIN_VERSION := 0.13.99
NAMECOIN_ARCHIVE := namecoin-${NAMECOIN_VERSION}-x86_64-linux-gnu.tar.gz
NAMECOIN_SHA256 := 294b1106001d6ea2b9d9ee6a655021ef207a24e8f1dec8efd5899728b3849129

all:
	wget https://namecoin.org/files/namecoin-core-${NAMECOIN_VERSION}-name-tab-beta1-notreproduced/${NAMECOIN_ARCHIVE}
	[ "$$(sha256sum ${NAMECOIN_ARCHIVE} | cut -d ' ' -f 1)" = "${NAMECOIN_SHA256}" ]
	tar -xf ${NAMECOIN_ARCHIVE}
	mv namecoin-${NAMECOIN_VERSION} namecoin
	rm namecoin/bin/namecoin-qt
	rm namecoin/bin/test_namecoin*


clean:
	rm -rf namecoin
	rm -f ${NAMECOIN_ARCHIVE}
