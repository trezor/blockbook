## BTC Testnet Setup
Get Bitcoin Core
```
wget https://bitcoin.org/bin/bitcoin-core-0.16.0/bitcoin-0.15.1-x86_64-linux-gnu.tar.gz
tar -xf bitcoin-0.16.0-x86_64-linux-gnu.tar.gz
```
Data are stored in */data/testnet*, in folders */data/testnet/bitcoin* for Bitcoin Core data, */data/testnet/blockbook* for Blockbook data.

Create configuration file */data/testnet/bitcoin/bitcoin.conf* with content
```
testnet=1
daemon=1
server=1
rpcuser=rpc
rpcpassword=rpc
rpcport=18030
txindex=1
```
Create script that starts the bitcoind daemon *run-testnet-bitcoind.sh* with increased rpcworkqueue and configured zeromq
```
#!/bin/bash

bitcoin-0.15.1/bin/bitcoind -datadir=/data/testnet/bitcoin -rpcworkqueue=32 -zmqpubhashtx=tcp://127.0.0.1:48330 -zmqpubhashblock=tcp://127.0.0.1:48330 -zmqpubrawblock=tcp://127.0.0.1:48330 -zmqpubrawtx=tcp://127.0.0.1:48330
```
Run the *run-testnet-bitcoind.sh* to get initial import of data.

Create blockchain configuration file */data/testnet/blockbook/btc-testnet.json*
```
{
  "rpcURL": "http://127.0.0.1:18030",
  "rpcUser": "rpc",
  "rpcPass": "rpc",
  "rpcTimeout": 25,
  "parse": true,
  "zeroMQBinding": "tcp://127.0.0.1:48330"
}
```

Create script that runs blockbook *run-testnet-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -coin=btc-testnet -blockchaincfg=/data/testnet/blockbook/btc-testnet.json  -datadir=/data/testnet/blockbook/db -sync -httpserver=:19030 -socketio=:19130 -certfile=server/testcert -explorer=https://testnet-bitcore1.trezor.io $1
```
To run blockbook with logging to file (run with nohup or daemonize or using screen)
```
./run-testnet-blockbook.sh 2>/data/testnet/blockbook/blockbook.log
```
