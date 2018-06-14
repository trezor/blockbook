DOGECOIN_VERSION := 1.10.0

all:
	wget https://github.com/dogecoin/dogecoin/releases/download/v${DOGECOIN_VERSION}/dogecoin-${DOGECOIN_VERSION}-linux64.tar.gz
	tar -xf dogecoin-${DOGECOIN_VERSION}-linux64.tar.gz
	mv dogecoin-${DOGECOIN_VERSION} dogecoin
	rm dogecoin/bin/dogecoin-qt
	rm dogecoin/bin/test_dogecoin*


clean:
	rm -rf dogecoin
	rm -f dogecoin-${DOGECOIN_VERSION}-linux64.tar.gz
