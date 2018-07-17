ZCASH_VERSION := 1.1.1
ZCASH_ARCHIVE := zcash-${ZCASH_VERSION}-linux64.tar.gz

all:
	wget https://z.cash/downloads/${ZCASH_ARCHIVE}
	wget https://z.cash/downloads/${ZCASH_ARCHIVE}.asc
	gpg --verify ${ZCASH_ARCHIVE}.asc
	tar -xf ${ZCASH_ARCHIVE}
	mv zcash-${ZCASH_VERSION} zcash

clean:
	rm -rf zcash
	rm -f ${ZCASH_ARCHIVE}
	rm -f ${ZCASH_ARCHIVE}.asc
