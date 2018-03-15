## Ethereum Testnet Setup
Get Ethereum
```
git clone https://github.com/ethereum/go-ethereum
cd go-ethereum/
make geth
```
Data are stored in */data/eth-testnet*, in folders */data/eth-testnet/eth* for Ethereum data, */data/eth-testnet/eth/blockbook* for Blockbook data.

Run geth with rpc and websocket interfaces, bound to all ip addresses - insecure! (run with nohup or daemonize or using screen)
```
go-ethereum/build/bin/geth --testnet --datadir /data/eth-testnet/eth --rpc --rpcport 18545 -rpcaddr 0.0.0.0 --rpccorsdomain "*" --ws --wsaddr 0.0.0.0 --wsport 18546 --wsorigins "*" 2>/data/eth-testnet/eth/eth.log
```

Create script that runs blockbook *run-eth-testnet-blockbook.sh*
```
#!/bin/bash

cd go/src/blockbook
./blockbook -path=/data/eth-testnet/blockbook/db -sync -rpcurl=ws://127.0.0.1:18546 -httpserver=:18555 -socketio=:18556 -certfile=server/testcert -coin=eth-testnet $1
```
To run blockbook with logging to file (run with nohup or daemonize or using screen)
```
./run-eth-testnet-blockbook.sh 2>/data/testnet/eth-testnet/blockbook.log
```
