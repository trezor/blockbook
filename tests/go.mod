module github.com/trezor/blockbook/tests

go 1.12

replace (
	github.com/trezor/blockbook/bchain  => ../bchain
	github.com/trezor/blockbook/bchain/coins  => ../bchain/coins
	github.com/trezor/blockbook/build/tools  => ../build/tools
	github.com/trezor/blockbook/tests/rpc  => ../tests/rpc
	github.com/trezor/blockbook/tests/sync  => ../tests/sync
)
