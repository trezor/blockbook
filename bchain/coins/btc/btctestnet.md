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
rpcport=18332
txindex=1
```
Create script that starts the bitcoind daemon *run-testnet-bitcoind.sh* with increased rpcworkqueue and configured zeromq
```
#!/bin/bash

bitcoin-0.15.1/bin/bitcoind -datadir=/data/testnet/bitcoin -rpcworkqueue=32 -zmqpubhashtx=tcp://127.0.0.1:18334 -zmqpubhashblock=tcp://127.0.0.1:18334 -zmqpubrawblock=tcp://127.0.0.1:18334 -zmqpubrawtx=tcp://127.0.0.1:18334
```
Run the *run-testnet-bitcoind.sh* to get initial import of data.

Create script that runs blockbook *run-testnet-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -path=/data/testnet/blockbook/db -sync -parse -rpcurl=http://127.0.0.1:18332 -httpserver=:18335 -socketio=:18336 -certfile=server/testcert -zeromq=tcp://127.0.0.1:18334 -explorer=https://testnet-bitcore1.trezor.io  -coin=btc-testnet $1
```
To run blockbook with logging to file (run with nohup or daemonize or using screen)
```
./run-testnet-blockbook.sh 2>/data/testnet/blockbook/blockbook.log
```
