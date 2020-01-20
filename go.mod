module github.com/trezor/blockbook

go 1.12

require (
	github.com/Groestlcoin/go-groestl-hash v0.0.0-20181012171753-790653ac190c // indirect
	github.com/allegro/bigcache v1.2.1 // indirect
	github.com/aristanetworks/goarista v0.0.0-20200214154357-2151774b0d85 // indirect
	github.com/dchest/blake256 v1.1.0 // indirect
	github.com/deckarep/golang-set v1.7.1 // indirect
	github.com/elastic/gosigar v0.10.5 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b
	github.com/juju/errors v0.0.0-20190930114154-d42613fe1ab9
	github.com/mr-tron/base58 v1.1.3 // indirect
	github.com/rs/cors v1.7.0 // indirect
	github.com/trezor/blockbook/api v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/bchain v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/bchain/coins v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/common v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/db v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/fiat v0.0.0-00010101000000-000000000000
	github.com/trezor/blockbook/server v0.0.0-00010101000000-000000000000
)

replace (
	github.com/trezor/blockbook/api => ./api
	github.com/trezor/blockbook/bchain => ./bchain
	github.com/trezor/blockbook/bchain/coins => ./bchain/coins
	github.com/trezor/blockbook/common => ./common
	github.com/trezor/blockbook/db => ./db
	github.com/trezor/blockbook/fiat => ./fiat
	github.com/trezor/blockbook/server => ./server
)
