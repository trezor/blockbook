GROESTLCOIN_VERSION := 2.16.0

all:
	wget https://github.com/Groestlcoin/groestlcoin/releases/download/v${GROESTLCOIN_VERSION}/groestlcoin-${GROESTLCOIN_VERSION}-x86_64-linux-gnu.tar.gz
	mkdir groestlcoin
	tar xvf groestlcoin-${GROESTLCOIN_VERSION}-x86_64-linux-gnu.tar.gz -C groestlcoin/
	rm groestlcoin/groestlcoin-qt

clean:
	rm -rf groestlcoin
	rm -f groestlcoin-${GROESTLCOIN_VERSION}-x86_64-linux-gnu.tar.gz
