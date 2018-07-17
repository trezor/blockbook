DASH_VERSION := 0.12.3
DASH_BUILD := .2
DASH_ARCHIVE := dashcore-${DASH_VERSION}${DASH_BUILD}-x86_64-linux-gnu.tar.gz

all:
	wget https://github.com/dashpay/dash/releases/download/v${DASH_VERSION}${DASH_BUILD}/${DASH_ARCHIVE}
	wget https://github.com/dashpay/dash/releases/download/v${DASH_VERSION}${DASH_BUILD}/SHA256SUMS.asc
	gpg --verify SHA256SUMS.asc
	sha256sum -c --ignore-missing SHA256SUMS.asc
	tar -xf ${DASH_ARCHIVE}
	mv dashcore-${DASH_VERSION} dash
	rm dash/bin/dash-qt
	rm dash/bin/test_dash


clean:
	rm -rf dash
	rm -f ${DASH_ARCHIVE}
	rm -f SHA256SUMS.asc
