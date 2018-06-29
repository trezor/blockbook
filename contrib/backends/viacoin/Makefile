VIACOIN_VERSION := 0.15.1

all:
	wget https://github.com/viacoin/viacoin/releases/download/${VIACOIN_VERSION}/viacoin-${VIACOIN_VERSION-x86_64-linux-gnu.tar.gz

	tar -xf viacoin-${VIACOIN_VERSION}-x86_64-linux-gnu.tar.gz
	mv viacoin-${VIACOIN_VERSION} viacoin
	rm viacoin/bin/viacoin-qt
	rm viacoin/bin/test_viacoin

clean:
	rm -rf viacoin
	rm -f viacoin-${VIACOIN_VERSION}-x86_64-linux-gnu.tar.gz
