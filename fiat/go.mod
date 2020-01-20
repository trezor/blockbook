module github.com/dmigwi/blockbook/fiat

go 1.12

require (
	github.com/bsm/go-vlq v0.0.0-20150828105119-ec6e8d4f5f4e // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9 // indirect
	github.com/tecbot/gorocksdb v0.0.0-20191217155057-f0fad39f321c // indirect
	github.com/trezor/blockbook v0.3.1
)

replace (
	github.com/trezor/blockbook/bchain  => ../bchain
	github.com/trezor/blockbook/bchain/coins/btc  => ../bchain/coins/btc
	github.com/trezor/blockbook/common  => ../common
	github.com/trezor/blockbook/db  => ../db
)
