## Zcash Setup
Get Zcash client
```
wget https://z.cash/downloads/zcash-1.0.15-linux64.tar.gz
tar xzf zcash-1.0.15-linux64.tar.gz
```

Run command to download the parameters used to create and verify shielded transactions:
```
zcash-1.0.15/bin/zcash-fetch-params
```

Data are stored in */data/zec* , in folders */data/zec/zcash* for Zcash client data, */data/zec/blockbook* for Blockbook data.

Create configuration file */data/zec/zcash/zcash.conf* with content
```
daemon=1
server=1
rpcuser=rpc
rpcpassword=rpc
rpcport=8032
txindex=1
mainnet=1
addnode=mainnet.z.cash
```

Create script *run-zec-zcashd.sh* that starts the zcashd daemon with increased rpcworkqueue and configured zeromq
```
#!/bin/bash

zcash-1.0.15/bin/zcashd -datadir=/data/zec/zcash -rpcworkqueue=32 -zmqpubhashblock=tcp://127.0.0.1:38332 -zmqpubrawblock=tcp://127.0.0.1:38332 -zmqpubhashtx=tcp://127.0.0.1:38332 -zmqpubrawtx=tcp://127.0.0.1:38332
```

Run the *run-zec-zcashd.sh* to get initial import of data.

Create blockchain configuration file */data/zec/blockbook/zec.json*
```
{
  "rpcURL": "http://127.0.0.1:8032",
  "rpcUser": "rpc",
  "rpcPass": "rpc",
  "rpcTimeout": 25,
  "parse": true,
  "zeroMQBinding": "tcp://127.0.0.1:38332"
}
```

Create *run-zec-blockbook.sh* script that starts blockbook
```
#!/bin/bash
./blockbook -coin=zec -blockchaincfg=/data/zec/blockbook/zec.json -datadir=/data/zec/blockbook/db -sync -internal=:9032 -public=:9132 -certfile=server/testcert -explorer=https://zec-bitcore1.trezor.io $1
```

To run blockbook with logging to file (run with nohup or daemonize using screen)
```
./run-zec-blockbook.sh 2> /data/zec/blockbook/blockbook.log
```
