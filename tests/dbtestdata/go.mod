module github.com/dmigwi/blockbook/tests/dbtestdata

go 1.12

require (
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9 // indirect
	github.com/pebbe/zmq4 v1.0.0 // indirect
	github.com/trezor/blockbook v0.3.1
)

replace github.com/trezor/blockbook/bchain  => ../../bchain
