GETH_VERSION := 1.8.10-eae63c51

all:
	wget https://gethstore.blob.core.windows.net/builds/geth-linux-amd64-${GETH_VERSION}.tar.gz
	tar -xf geth-linux-amd64-${GETH_VERSION}.tar.gz
	mv geth-linux-amd64-${GETH_VERSION} ethereum

clean:
	rm -rf ethereum
	rm -f geth-linux-amd64-${GETH_VERSION}.tar.gz*
