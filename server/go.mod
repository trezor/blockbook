module github.com/trezor/blockbook/server

go 1.12

require (
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/gorilla/websocket v1.4.1 // indirect
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9 // indirect
	github.com/martinboehm/golang-socketio v0.0.0-20180414165752-f60b0a8befde // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.1 // indirect
	github.com/prometheus/client_golang v1.3.0 // indirect

)

replace (
	github.com/trezor/blockbook/api => ../api
	github.com/trezor/blockbook/bchain => ../bchain
	github.com/trezor/blockbook/bchain/coins/btc => ../bchain/coins/btc
	github.com/trezor/blockbook/common => ../common
	github.com/trezor/blockbook/db => ../db
	github.com/trezor/blockbook/tests/dbtestdata => ../tests/dbtestdata
)
