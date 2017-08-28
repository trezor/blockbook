# blockbook

## Install

Setup go environment:

```
sudo apt-get install golang
sudo apt-get install git
go help gopath
```

Install RocksDB: https://github.com/facebook/rocksdb/blob/master/INSTALL.md

```
sudo apt-get install libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev liblz4-dev libzstd-dev
cd /path/to/rocksdb
make static_lib
```

Install gorocksdb: https://github.com/tecbot/gorocksdb

```
CGO_CFLAGS="-I/path/to/rocksdb/include" \
CGO_LDFLAGS="-L/path/to/rocksdb -lrocksdb -lstdc++ -lm -lz -lbz2 -lsnappy -llz4 -lzstd" \
  go get github.com/tecbot/gorocksdb
```

Install blockbook:

```
go get github.com/jpochyla/blockbook
```

## Usage

```
$GOPATH/bin/blockbook --help
```