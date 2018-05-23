## BTC Setup
Get Bitcoin Core
```
wget https://bitcoin.org/bin/bitcoin-core-0.16.0/bitcoin-0.15.1-x86_64-linux-gnu.tar.gz
tar -xf bitcoin-0.16.0-x86_64-linux-gnu.tar.gz
```

Data are stored in */data/btc*, in folders */data/btc/bitcoin* for Bitcoin Core data, */data/btc/blockbook* for Blockbook data.

Create configuration file */data/btc/bitcoin/bitcoin.conf* with content
```
daemon=1
server=1
rpcuser=rpc
rpcpassword=rpc
rpcport=8030
txindex=1
```
Create script that starts the bitcoind daemon *run-btc-bitcoind.sh* with increased rpcworkqueue and configured zeromq
```
#!/bin/bash

bitcoin-0.15.1/bin/bitcoind -datadir=/data/btc/bitcoin -rpcworkqueue=32 -zmqpubhashtx=tcp://127.0.0.1:38330 -zmqpubhashblock=tcp://127.0.0.1:38330 -zmqpubrawblock=tcp://127.0.0.1:38330 -zmqpubrawtx=tcp://127.0.0.1:38330
```
Run the *run-btc-bitcoind.sh* to get initial import of data.

Create blockchain configuration file */data/testnet/blockbook/btc.json*
```
{
  "rpcURL": "http://127.0.0.1:8030",
  "rpcUser": "rpc",
  "rpcPass": "rpc",
  "rpcTimeout": 25,
  "parse": true,
  "zeroMQBinding": "tcp://127.0.0.1:38330"
}
```

Create script that runs blockbook *run-btc-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -coin=btc -blockchaincfg=/data/btc/blockbook/btc.json -datadir=/data/btc/blockbook/db -sync -httpserver=:9030 -socketio=:9130 -certfile=server/testcert -explorer=https://bitcore1.trezor.io/ $1
```
To run blockbook with logging to file  (run with nohup or daemonize or using screen)
```
./run-btc-blockbook.sh 2>/data/btc/blockbook/blockbook.log
```

