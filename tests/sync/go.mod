module github.com/trezor/blockbook //tests/sync

go 1.12

replace (
	github.com/trezor/blockbook/bchain  => ../../bchain
	github.com/trezor/blockbook/common  => ../../common
	github.com/trezor/blockbook/db  => ../../db
)
