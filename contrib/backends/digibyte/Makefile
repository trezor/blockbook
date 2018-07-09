DIGIBYTE_VERSION := 6.16.2

all:
	wget https://github.com/digibyte/digibyte/releases/download/v${DIGIBYTE_VERSION}/digibyte-${DIGIBYTE_VERSION}-x86_64-linux-gnu.tar.gz
	tar xvf digibyte-${DIGIBYTE_VERSION}-x86_64-linux-gnu.tar.gz
	mv digibyte-${DIGIBYTE_VERSION} digibyte

clean:
	rm -rf digibyte
	rm -f digibyte-${DIGIBYTE_VERSION}-x86_64-linux-gnu.tar.gz


