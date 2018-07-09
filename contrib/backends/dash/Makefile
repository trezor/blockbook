DASH_VERSION := 0.12.2
DASH_BUILD := .3

all:
	wget https://github.com/dashpay/dash/releases/download/v${DASH_VERSION}${DASH_BUILD}/dashcore-${DASH_VERSION}${DASH_BUILD}-linux64.tar.gz
	tar -xf dashcore-${DASH_VERSION}${DASH_BUILD}-linux64.tar.gz
	mv dashcore-${DASH_VERSION} dash
	rm dash/bin/dash-qt
	rm dash/bin/test_dash


clean:
	rm -rf dash
	rm -f dashcore-${DASH_VERSION}${DASH_BUILD}-linux64.tar.gz
