LITECOIN_VERSION := 0.16.0

all:
	wget https://download.litecoin.org/litecoin-${LITECOIN_VERSION}/linux/litecoin-${LITECOIN_VERSION}-x86_64-linux-gnu.tar.gz
	tar -xf litecoin-${LITECOIN_VERSION}-x86_64-linux-gnu.tar.gz
	mv litecoin-${LITECOIN_VERSION} litecoin
	rm litecoin/bin/litecoin-qt
	rm litecoin/bin/test_litecoin


clean:
	rm -rf litecoin
	rm -f litecoin-${LITECOIN_VERSION}-x86_64-linux-gnu.tar.gz
