BGOLD_VERSION := 0.15.1
BUILD_VERSION :=
BGOLD_ARCHIVE := bitcoin-gold-${BGOLD_VERSION}-x86_64-linux-gnu.tar.gz

all:
	wget https://github.com/BTCGPU/BTCGPU/releases/download/v${BGOLD_VERSION}${BUILD_VERSION}/${BGOLD_ARCHIVE}
	wget https://github.com/BTCGPU/BTCGPU/releases/download/v${BGOLD_VERSION}/SHA256SUMS.asc
	gpg --verify SHA256SUMS.asc
	sha256sum -c --ignore-missing SHA256SUMS.asc
	tar -xf ${BGOLD_ARCHIVE}
	mv bitcoin-gold-${BGOLD_VERSION} bgold
	rm bgold/bin/bitcoin-qt
	rm bgold/bin/test_bitcoin

clean:
	rm -rf bgold
	rm -f ${BGOLD_ARCHIVE}
	rm -f SHA256SUMS.asc
