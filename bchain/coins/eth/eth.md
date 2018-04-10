## Ethereum Testnet Setup
Get Ethereum
```
git clone https://github.com/ethereum/go-ethereum
cd go-ethereum/
make geth
```
Data are stored in */data/eth*, in folders */data/eth/eth* for Ethereum data, */data/eth/blockbook* for Blockbook data.

Run geth with rpc and websocket interfaces, bound to all ip addresses - insecure! (run with nohup or daemonize or using screen)
```
go-ethereum/build/bin/geth --syncmode "full" --cache 1024 --datadir /data/eth/eth --port "35555" --rpc --rpcport 8545 -rpcaddr 0.0.0.0 --rpccorsdomain "*" --ws --wsaddr 0.0.0.0 --wsport 8546 --wsorigins "*" 2>/data/eth/eth/eth.log
```

Create script that runs blockbook *run-eth-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -coin=eth -blockchaincfg=/data/eth/blockbook/eth.json -datadir=/data/eth/blockbook/db -sync -httpserver=:8555 -socketio=:8556 -certfile=server/testcert  $1
```
To run blockbook with logging to file (run with nohup or daemonize or using screen)
```
./run-eth-blockbook.sh 2>/data/eth/blockbook/blockbook.log
```
