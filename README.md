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

Install ZeroMQ: https://github.com/zeromq/libzmq

Install Go interface to ZeroMQ:
```
go get github.com/pebbe/zmq4
```

Install blockbook:

```
go get github.com/jpochyla/blockbook
```

## Usage

```
$GOPATH/bin/blockbook --help
```

# Data storage in RocksDB

Blockbook stores data the following column families

- **default**

  at the moment not used, will store statistical data etc.

- **height** - mapping of *block height* to *block hash*

  *Block heigh* stored as binary array of 4 bytes (big endian uint32)  
  *Block hash* stored as binary array of 32 bytes

  Example - the first four blocks (all data hex encoded)
```
0x00000000 : 0x000000000933EA01AD0EE984209779BAAEC3CED90FA3F408719526F8D77F4943
0x00000001 : 0x00000000B873E79784647A6C82962C70D228557D24A747EA4D1B8BBE878E1206
0x00000002 : 0x000000006C02C8EA6E4FF69651F7FCDE348FB9D557A06E6957B65552002A7820
0x00000003 : 0x000000008B896E272758DA5297BCD98FDC6D97C9B765ECEC401E286DC1FDBE10
```

- **outputs** -  mapping of *address+block height* to *array of outpoints*

  *Address+block height* stored as binary array 25 bytes total - 21 bytes address without checksum + 4 bytes (big endian uint32) block height  
  *array of outpoints* stored as binary array of 32 bytes for address + variable length outpoint number for each outpoint

  Example - (all data hex encoded)
```
0x6FCB8DBDD6F207F559162EF3BBD4229911DA248C3000000029 : 0xC459D961CC607A12AFDC6A8A200E84F1AD8E2021C2745FE12E0231DD90CA46BE00
0x6FCF12C87234AF9C402246B1A5D5C3F937337E6ECC00000013 : 0x73A4988ADF462B6540CFA59097804174B298CFA439F73C1A072C2C6FBDBE57C700
0x6FD3F80654BDA100BB704FBAF49E759E6084AB10DD0000005D : 0xC12A5ACAE3EFD788F088FCFA6D267ED48681A1AE4D2F477BFA60C0500274B6BE00
0x6FD5D788B43528CF8AF3EFBB050060B1BE26A54B4600000033 : 0x67F41C86F443AC7E00F494DD6B5DB2E999AA64E77A537728096CE4AB32F3170D00
```

- **inputs** 
  
  tbd