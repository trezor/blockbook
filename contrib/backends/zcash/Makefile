ZCASH_VERSION := 1.0.15

all:
	wget https://z.cash/downloads/zcash-${ZCASH_VERSION}-linux64.tar.gz
	tar -xf zcash-${ZCASH_VERSION}-linux64.tar.gz
	mv zcash-${ZCASH_VERSION} zcash
	rm zcash/bin/zcash-tx

clean:
	rm -rf zcash
	rm -f zcash-${ZCASH_VERSION}-linux64.tar.gz*
