DOGECOIN_VERSION := 1.10.0
DOGECOIN_ARCHIVE := dogecoin-${DOGECOIN_VERSION}-linux64.tar.gz
DOGECOIN_SHA256 := 2e5b61842695d74ebcd30f21014cf74b6265f0f7756e9f140f031259bb3cd656

all:
	wget https://github.com/dogecoin/dogecoin/releases/download/v${DOGECOIN_VERSION}/${DOGECOIN_ARCHIVE}
	[ "$$(sha256sum ${DOGECOIN_ARCHIVE} | cut -d ' ' -f 1)" = "${DOGECOIN_SHA256}" ]
	tar -xf ${DOGECOIN_ARCHIVE}
	mv dogecoin-${DOGECOIN_VERSION} dogecoin
	rm dogecoin/bin/dogecoin-qt
	rm dogecoin/bin/test_dogecoin*


clean:
	rm -rf dogecoin
	rm -f ${DOGECOIN_ARCHIVE}
