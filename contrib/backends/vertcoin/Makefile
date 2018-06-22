VERTCOIN_VERSION := 0.13.2

all:
	wget https://github.com/vertcoin-project/vertcoin-core/releases/download/${VERTCOIN_VERSION}/vertcoind-v${VERTCOIN_VERSION}-linux-amd64.zip
	mkdir vertcoin
	unzip vertcoind-v${VERTCOIN_VERSION}-linux-amd64.zip -d vertcoin/

clean:
	rm -rf vertcoin
	rm -f vertcoind-v${VERTCOIN_VERSION}-linux-amd64.zip
