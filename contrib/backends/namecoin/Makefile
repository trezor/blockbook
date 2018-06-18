NAMECOIN_VERSION := 0.13.99

all:
	wget https://namecoin.org/files/namecoin-core-${NAMECOIN_VERSION}-name-tab-beta1-notreproduced/namecoin-${NAMECOIN_VERSION}-x86_64-linux-gnu.tar.gz
	tar -xf namecoin-${NAMECOIN_VERSION}-x86_64-linux-gnu.tar.gz
	mv namecoin-${NAMECOIN_VERSION} namecoin
	rm namecoin/bin/namecoin-qt
	rm namecoin/bin/test_namecoin*


clean:
	rm -rf namecoin
	rm -f namecoin-${NAMECOIN_VERSION}-x86_64-linux-gnu.tar.gz
